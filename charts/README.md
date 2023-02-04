# Lint

    helm lint ./charts/gateboard --values charts/gateboard/values.yaml

    helm lint ./charts/gateboard-discovery --values charts/gateboard-discovery/values.yaml

# Debug

    helm template ./charts/gateboard --values charts/gateboard/values.yaml --debug

    helm template ./charts/gateboard-discovery --values charts/gateboard-discovery/values.yaml --debug
