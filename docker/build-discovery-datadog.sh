#!/bin/bash

app=gateboard-discovery

version=$(go run ./cmd/$app -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

dd=-datadog

echo version=$version

docker build --no-cache \
    -t udhos/$app:latest${dd} \
    -t udhos/$app:$version${dd} \
    -f docker/Dockerfile.discovery.datadog .

echo push:
echo "docker push udhos/$app:$version${dd}; docker push udhos/$app:latest${dd}" > docker-push-discovery-datadog.sh
chmod a+rx docker-push-discovery-datadog.sh
echo docker-push-discovery-datadog.sh:
cat docker-push-discovery-datadog.sh
