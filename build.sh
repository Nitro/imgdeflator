#!/bin/bash

die() {
    echo $1
    exit 1
}

CURRENT_REVISION=$(git rev-parse --short HEAD)

docker build -t gonitro/imgproxy:${CURRENT_REVISION} -t gonitro/imgproxy:latest . || die "Failed to build container"
docker push gonitro/imgproxy:${CURRENT_REVISION}
docker push gonitro/imgproxy:latest