package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3/s3manager"
	"github.com/davidbyttow/govips/pkg/vips"
	"github.com/hashicorp/golang-lru"
	log "github.com/sirupsen/logrus"
)

const (
	MaxUploadSize     = 5 * 1024 * 1024 //5MB
	HTTPPort          = "8080"
	UploadTimeout     = 10 * time.Second
	RequestTimeout    = 11 * time.Second // Make sure this is a bit bigger than the UploadTimeout
	UploaderCacheSize = 25
	DefaultS3Region   = "eu-central-1"
	MaxWidth          = 4096
	MaxHeight         = 4096
)

var (
	// TODO: Check for errors when UploaderCacheSize < 0
	uploaderCache, _ = lru.New(UploaderCacheSize)
)

// getS3Uploader looks up an S3 bucket in the uploaderCache and returns a configured
// s3manager.Uploader for it or provisions a new one and returns that.
func getS3Uploader(ctx context.Context, bucket string) (*s3manager.Uploader, error) {
	if uploader, ok := uploaderCache.Get(bucket); ok {
		return uploader.(*s3manager.Uploader), nil
	}

	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return nil, fmt.Errorf("could not load the default AWS config: %s", err)
	}

	region, err := s3manager.GetBucketRegion(ctx, cfg, bucket, DefaultS3Region)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NotFound" {
			return nil, fmt.Errorf("region for bucket %q not found", bucket)
		}
		return nil, fmt.Errorf("failed to determine region for bucket %q: %s", bucket, err)
	}
	log.Debugf("Bucket %q is in region: %s", bucket, region)

	cfg.Region = region
	uploader := s3manager.NewUploader(cfg)

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

func resizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Debugf("Method %q not allowed", r.Method)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if r.ContentLength > MaxUploadSize {
		log.Debugf("File too large (%d bytes)", r.ContentLength)
		http.Error(w, fmt.Sprintf("File too large (%d bytes)", r.ContentLength), http.StatusRequestEntityTooLarge)
		return
	}

	query := r.URL.Query()
	width := parseUintValue(query.Get("width"), MaxWidth)
	height := parseUintValue(query.Get("height"), MaxHeight)
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

	uploader, err := getS3Uploader(r.Context(), s3URL.Host)
	if err != nil {
		log.Warnf("Failed to get uploader for bucket %q: %s", s3URL.Host, err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Set a hard limit for how much we can read from the body
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)

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

func main() {
	// Start vips and disable caching, because I think we won't benefit much from it
	// Details: https://github.com/DarthSim/imgproxy/blob/a344a47f0fa4b492e0a54db047a53991c05419ac/process.go#L52
	vips.Startup(&vips.Config{
		// TODO: See if we want to enable file caching later
		MaxCacheFiles: 1,
		MaxCacheSize:  1,
		MaxCacheMem:   1,
	})
	defer vips.Shutdown()

	ctx := initGracefulStop()

	http.Handle("/", http.TimeoutHandler(http.HandlerFunc(resizeHandler), UploadTimeout, "Upload timeout"))

	srv := &http.Server{
		Addr:         ":" + HTTPPort,
		ReadTimeout:  RequestTimeout,
		WriteTimeout: RequestTimeout,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Errorf("http.ListenAndServe error: %s")
		}
	}()

	// Wait for shutdown signal
	_ = <-ctx.Done()

	// Shutdown server gracefully
	ctx, done := context.WithTimeout(context.Background(), UploadTimeout)
	defer done()
	err := srv.Shutdown(ctx)
	if err != nil {
		log.Fatalf("HTTP server exited with error: %s", err)
	}
}
