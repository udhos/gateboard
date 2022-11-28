#!/bin/bash

version=$(go run ./cmd/gateboard -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

# Multiarch

docker buildx build \
   --push \
   --tag udhos/gateboard:latest \
   --tag udhos/gateboard:$version \
   --platform linux/amd64,linux/arm64 \
   -f ./docker/Dockerfile.multiarch .

# docker buildx imagetools inspect udhos/gateboard:latest

