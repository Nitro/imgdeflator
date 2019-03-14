package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Nitro/urlsign"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3/s3manager"
	"github.com/davidbyttow/govips/pkg/vips"
	"github.com/hashicorp/golang-lru"
	"github.com/relistan/envconfig"
	"github.com/relistan/rubberneck"
	log "github.com/sirupsen/logrus"
)

const (
	UploaderCacheSize = 25
)

var (
	// TODO: Check for errors when UploaderCacheSize < 0
	uploaderCache, _ = lru.New(UploaderCacheSize)
)

type Config struct {
	LoggingLevel      string        `envconfig:"LOGGING_LEVEL" default:"info"`
	MaxUploadSize     int64         `envconfig:"MAX_UPLOAD_SIZE" default:"5242880"` //5MB
	HTTPPort          string        `envconfig:"HTTP_PORT" default:"8080"`
	UploadTimeout     time.Duration `envconfig:"UPLOAD_TIMEOUT" default:"10s"`
	RequestTimeout    time.Duration `envconfig:"REQUEST_TIMEOUT" default:"11s"`
	DefaultS3Region   string        `envconfig:"DEFAULT_S3_REGION" default:"eu-central-1"`
	MaxWidth          uint64        `envconfig:"MAX_WIDTH" default:"4096"`
	MaxHeight         uint64        `envconfig:"MAX_HEIGHT" default:"4096"`
	UrlSigningSecret  string        `envconfig:"URL_SIGNING_SECRET" default:"deadbeef"`
	SigningBucketSize time.Duration `envconfig:"SIGNING_BUCKET_SIZE" default:"8h"`
}

func configureLoggingLevel(config *Config) {
	switch config.LoggingLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}

// getS3Uploader looks up an S3 bucket in the uploaderCache and returns a configured
// s3manager.Uploader for it or provisions a new one and returns that.
func getS3Uploader(ctx context.Context, bucket, defaultRegion string) (*s3manager.Uploader, error) {
	if uploader, ok := uploaderCache.Get(bucket); ok {
		return uploader.(*s3manager.Uploader), nil
	}

	awsCfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return nil, fmt.Errorf("could not load the default AWS config: %s", err)
	}

	region, err := s3manager.GetBucketRegion(ctx, awsCfg, bucket, defaultRegion)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NotFound" {
			return nil, fmt.Errorf("region for bucket %q not found", bucket)
		}
		return nil, fmt.Errorf("failed to determine region for bucket %q: %s", bucket, err)
	}
	log.Debugf("Bucket %q is in region: %s", bucket, region)

	awsCfg.Region = region
	uploader := s3manager.NewUploader(awsCfg)

	// Don't overwrite a cached entry that got written by another goroutine in the mean time
	_, _ = uploaderCache.ContainsOrAdd(bucket, uploader)

	return uploader, nil
}

func decodePath(path string) (string, error) {
	decodedPath, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(path, "/"))
	if err != nil {
		return "", err
	}

	return string(decodedPath), nil
}

func parseS3URL(s3URL string) (*url.URL, error) {
	u, err := url.Parse(s3URL)
	if err != nil {
		return nil, fmt.Errorf("Invalid S3 URL: %s", err)
	}

	return u, nil
}

func parseUintValue(value string, maxValue uint64) uint64 {
	if value != "" {
		parsedValue, err := strconv.ParseUint(value, 10, 32)
		if err != nil || parsedValue > maxValue {
			return 0
		}

		return parsedValue
	}
	return 0
}

type Clock interface {
	Now() time.Time
}

type utcClock struct {
}

func (c *utcClock) Now() time.Time {
	return time.Now().UTC()
}

type Resizer struct {
	config *Config
	clock  Clock
}

func NewResizer(config *Config) *Resizer {
	return &Resizer{config: config, clock: &utcClock{}}
}

func (resizer *Resizer) Handler(w http.ResponseWriter, r *http.Request) {
	log.Infof("Received resize request: %s", r.URL)

	if r.Method != http.MethodPost {
		log.Debugf("Method %q not allowed", r.Method)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if r.ContentLength > resizer.config.MaxUploadSize {
		log.Debugf("File too large (%d bytes)", r.ContentLength)
		http.Error(w, fmt.Sprintf("File too large (%d bytes)", r.ContentLength), http.StatusRequestEntityTooLarge)
		return
	}

	if resizer.config.UrlSigningSecret != "" &&
		!urlsign.IsValidSignature(
			resizer.config.UrlSigningSecret,
			resizer.config.SigningBucketSize,
			resizer.clock.Now(),
			r.URL.String(),
		) {
		log.Debugf("Invalid URL signature: %s", r.URL)
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	query := r.URL.Query()
	width := parseUintValue(query.Get("width"), resizer.config.MaxWidth)
	height := parseUintValue(query.Get("height"), resizer.config.MaxHeight)
	if width == 0 && height == 0 {
		log.Debugf("Invalid width/height (%q/%q)", query.Get("width"), query.Get("height"))
		http.Error(
			w,
			fmt.Sprintf("Invalid width/height (%q/%q)", query.Get("width"), query.Get("height")),
			http.StatusBadRequest,
		)
		return
	}

	decodedPath, err := decodePath(r.URL.Path)
	if err != nil {
		log.Debugf("Failed to extract s3 URL from path %q: %s", r.URL.Path, err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	s3URL, err := parseS3URL(decodedPath)
	if err != nil {
		log.Debugf("Failed to extract s3 bucket from URL %q: %s", decodedPath, err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	uploader, err := getS3Uploader(r.Context(), s3URL.Host, resizer.config.DefaultS3Region)
	if err != nil {
		log.Warnf("Failed to get uploader for bucket %q: %s", s3URL.Host, err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Set a hard limit for how much we can read from the body
	r.Body = http.MaxBytesReader(w, r.Body, resizer.config.MaxUploadSize)

	// Resize image
	imageTransform := vips.NewTransform().Load(r.Body)

	if width > 0 {
		imageTransform.ResizeWidth(int(width))
	}
	if height > 0 {
		imageTransform.ResizeHeight(int(height))
	}

	buf, _, err := imageTransform.Apply()
	if err != nil {
		log.Warnf("Failed to resize image for URL %q: %s", s3URL.String(), err)
		http.Error(w, "Internal error", http.StatusServiceUnavailable)
		return
	}

	_, err = uploader.UploadWithContext(
		r.Context(),
		&s3manager.UploadInput{
			Body:        bytes.NewReader(buf),
			Bucket:      aws.String(s3URL.Host),
			ContentType: aws.String(r.Header.Get("Content-Type")),
			Key:         aws.String(strings.TrimPrefix(s3URL.Path, "/")),
		},
	)
	if err != nil {
		log.Warnf("Failed to upload %q: %s", s3URL.String(), err)
		http.Error(w, "Internal error", http.StatusServiceUnavailable)
		return
	}
}

func initGracefulStop() context.Context {
	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sig := <-gracefulStop
		log.Warnf("Received signal %q. Exiting as soon as possible!", sig)
		cancel()
	}()

	return ctx
}

func healthHandler(response http.ResponseWriter, _ *http.Request) {
	type HealthPayload struct {
		Message string
	}

	response.Header().Set("Content-Type", "application/json")

	message, _ := json.Marshal(HealthPayload{Message: "Healthy!"})

	fmt.Fprint(response, string(message))
}

// corsHandler sets the appropriate CORS headers in a closure
// which wraps the specified handler
func corsHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")

		// For OPTIONS requests, we just forward the Access-Control-Request-Headers as
		// Access-Control-Allow-Headers in the reply and return
		if r.Method == http.MethodOptions {
			if headers, ok := r.Header["Access-Control-Request-Headers"]; ok {
				for _, header := range headers {
					w.Header().Add("Access-Control-Allow-Headers", header)
				}
			}

			return
		}

		handler(w, r)
	}
}

func main() {
	var config Config
	err := envconfig.Process("imgdeflator", &config)
	if err != nil {
		log.Fatalf("Failed to parse the configuration parameters: %s", err)
	}

	configureLoggingLevel(&config)

	rubberneck.Print(&config)

	if config.UrlSigningSecret == "" {
		log.Warn("No URL signing secret was set. Running in insecure mode!")
	}

	// Start vips and disable caching, because I think we won't benefit much from it
	// Details: https://github.com/DarthSim/imgproxy/blob/a344a47f0fa4b492e0a54db047a53991c05419ac/process.go#L52
	vips.Startup(&vips.Config{
		// TODO: See if we want to enable file caching later
		MaxCacheFiles: 1,
		MaxCacheSize:  1,
		MaxCacheMem:   1,
	})
	defer vips.Shutdown()

	resizer := NewResizer(&config)
	http.Handle("/", http.TimeoutHandler(corsHandler(resizer.Handler), config.UploadTimeout, "Upload timeout"))
	http.HandleFunc("/health", healthHandler)

	srv := &http.Server{
		Addr:         ":" + config.HTTPPort,
		ReadTimeout:  config.RequestTimeout,
		WriteTimeout: config.RequestTimeout,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Errorf("http.ListenAndServe error: %s")
		}
	}()

	ctx := initGracefulStop()

	// Wait for shutdown signal
	_ = <-ctx.Done()

	// Shutdown server gracefully
	ctx, done := context.WithTimeout(context.Background(), config.UploadTimeout)
	defer done()
	err = srv.Shutdown(ctx)
	if err != nil {
		log.Fatalf("HTTP server exited with error: %s", err)
	}
}
