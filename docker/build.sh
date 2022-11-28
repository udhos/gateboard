#!/bin/bash

version=$(go run ./cmd/gateboard -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

docker build \
    -t udhos/gateboard:latest \
    -t udhos/gateboard:$version \
    -f docker/Dockerfile .
