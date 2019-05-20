# imgdeflator

Imgdeflator is a small web server which handles `POST` requests with image attachments. It passes the received image through [`libvips`](https://jcupitt.github.io/libvips/) to resize it to the specified width/height and then it stores it on S3 at the specified path.

It uses [urlsign](https://github.com/Nitro/urlsign) to validate the signature `token` in the request URL.

Kudos to [DarthSim](https://github.com/DarthSim) for writing [imgproxy](https://github.com/DarthSim/imgproxy), which served as inspiration for this library!

## Building imgdeflator locally

```shell
go get github.com/Nitro/imgdeflator
```

## Running imgdeflator locally

Just run the executable. By default, it will bind to port `8080` and handle POST requests in the following format:

```
http://127.0.0.1:8080/base64_encoded_s3_location?width=1024&token=valid_token
```
or
```
http://127.0.0.1:8080/base64_encoded_s3_location?height=768&token=valid_token
```

Configuration is done using environment variables:

- `IMGDEFLATOR_LOGGING_LEVEL`: The cut off level for log messages. Accepted values: `debug`, `info`, `warn`, `error` (default `info`).
- `IMGDEFLATOR_MAX_UPLOAD_SIZE`: The maximum allowed size for the `POST`ed image (default `5242880` which is 5MB).
- `IMGDEFLATOR_HTTP_PORT`: The port to listen on for HTTP connections (default `8080`).
- `IMGDEFLATOR_UPLOAD_TIMEOUT`: The maximum allowed processing duration of the HTTP handler before sending an error to the user (default `10s`).
- `IMGDEFLATOR_REQUEST_TIMEOUT`: The maximum allowed duration of the entire HTTP request before sending an error to the user (default `11s`).
- `IMGDEFLATOR_DEFAULT_S3_REGION`: The default S3 region where to look for the S3 bucket of the received S3 location (default `eu-central-1`).
- `IMGDEFLATOR_MAX_WIDTH`: The maximum `POST`ed image width (default `4096`).
- `IMGDEFLATOR_MAX_HEIGHT`: The maximum `POST`ed image height (default `4096`).
- `IMGDEFLATOR_URL_SIGNING_SECRET`: A secret to use when validating signed URLs (default: `deadbeef`). Set it to empty string to disable signature validation.
- `IMGDEFLATOR_SIGNING_BUCKET_SIZE`: The `urlsign` time bucket size (default `8h`). It provides a `3*bucketSize` window of validity for each signature. See the [`urlsign`](https://github.com/Nitro/urlsign) documentation for more information.

## Testing imgdeflator locally

- base64-encode a valid S3 location where you wish the image to be stored and append that to the imgdeflator URL:

```Shell
> echo -n "s3://nitro-junk/imgdeflator.jpg" | base64 | tr '=' '\0' | xargs -I {} echo "http://127.0.0.1:8080/{}"
http://127.0.0.1:8080/czM6Ly9uaXRyby1qdW5rL2ltZ2RlZmxhdG9yLmpwZw
```

- Instruct imgdeflator to shrink `resources/tweety.jpg` to have `width=1024` and store it at `s3://nitro-junk/imgdeflator.jpg`:

```Shell
> curl -v -H "Content-Type: image/jpeg" --data-binary "@resources/tweety.jpg" "http://127.0.0.1:8080/czM6Ly9uaXRyby1qdW5rL2ltZ2RlZmxhdG9yLmpwZw?width=1024"
```

# Copyright

Copyright (c) 2019 Nitro Software.
