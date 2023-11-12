#!/bin/bash

# the ./docs dir is published as https://udhos.github.io/gateboard/

chart_url=https://udhos.github.io/gateboard/

gen_chart() {

    local chart_dir="$1"

    # generate new chart package from source into ./docs
    helm package $chart_dir -d ./docs

    #
    # copy new chart into ./charts-tmp
    #

    local chart_name=$(gojq --yaml-input -r .name < $chart_dir/Chart.yaml)
    local chart_version=$(gojq --yaml-input -r .version < $chart_dir/Chart.yaml)
    local chart_pkg=${chart_name}-${chart_version}.tgz

    cp docs/${chart_pkg} charts-tmp
}

rm -rf charts-tmp
mkdir -p charts-tmp

gen_chart charts/gateboard
gen_chart charts/gateboard-discovery

#
# merge new chart index into docs/index.yaml
#

git checkout docs/index.yaml ;# reset index

# regenerate the index from existing chart packages
helm repo index charts-tmp --url $chart_url --merge docs/index.yaml

# new merged chart index was generated as ./charts-tmp/index.yaml,
# copy it back to ./docs
cp charts-tmp/index.yaml docs

echo "#"
echo "# check that ./docs is fine then:"
echo "#"
echo "git add docs"
echo "git commit -m 'Update chart repository.'"
echo "git push"
