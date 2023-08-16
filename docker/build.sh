#!/bin/bash

version=$(go run ./cmd/gateboard -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

docker build \
    --no-cache \
    -t udhos/gateboard:latest \
    -t udhos/gateboard:$version \
    -f docker/Dockerfile .

echo "push: docker push udhos/gateboard:$version; docker push udhos/gateboard:latest"
