
version=$(go run ./cmd/gateboard -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

version_disc=$(go run ./cmd/gateboard-discovery -version | awk '{ print $2 }' | awk -F= '{ print $2 }')

echo git tag v${version}
echo git tag discovery${version_disc}
echo git tag chart-gateboard${version}
echo git tag chart-gateboard-discovery${version_disc}
