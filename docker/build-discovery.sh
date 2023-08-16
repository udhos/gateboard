#!/bin/bash

version=$(go run ./cmd/gateboard-discovery -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

docker build \
    --no-cache \
    -t udhos/gateboard-discovery:latest \
    -t udhos/gateboard-discovery:$version \
    -f docker/Dockerfile.discovery .

echo "push: docker push udhos/gateboard-discovery:$version; docker push udhos/gateboard-discovery:latest"
