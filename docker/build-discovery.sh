#!/bin/bash

version=$(go run ./cmd/gateboard-discovery -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo version=$version

docker build \
    --no-cache \
    -t udhos/gateboard-discovery:latest \
    -t udhos/gateboard-discovery:$version \
    -f docker/Dockerfile.discovery .

echo push:
echo "docker push udhos/gateboard-discovery:$version; docker push udhos/gateboard-discovery:latest" > docker-push-discovery.sh
chmod a+rx docker-push-discovery.sh
echo docker-push-discovery.sh:
cat docker-push-discovery.sh
