#!/bin/bash

# the ./docs dir is published as https://udhos.github.io/gateboard/

# generate chart package from source
helm package ./charts/gateboard ./charts/gateboard-discovery -d ./docs

# regenerate the index from existing chart packages
helm repo index ./docs --url https://udhos.github.io/gateboard/

echo "#"
echo "# check that ./docs is fine then:"
echo "#"
echo "git add docs"
echo "git commit -m 'Update chart repository.'"
echo "git push"