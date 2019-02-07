#!/bin/bash

die() {
    echo $1
    exit 1
}

CURRENT_REVISION=$(git rev-parse --short HEAD)

docker build -t gonitro/imgdeflator:${CURRENT_REVISION} -t gonitro/imgdeflator:latest . || die "Failed to build container"
docker push gonitro/imgdeflator:${CURRENT_REVISION}
docker push gonitro/imgdeflator:latest