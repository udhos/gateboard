# Lint

    helm lint ./charts/gateboard --values charts/gateboard/values.yaml

    helm lint ./charts/gateboard-discovery --values charts/gateboard-discovery/values.yaml

# Debug

    helm template ./charts/gateboard --values charts/gateboard/values.yaml --debug

    helm template ./charts/gateboard-discovery --values charts/gateboard-discovery/values.yaml --debug

# Render at server

    helm install my-gateboard ./charts/gateboard --values charts/gateboard/values.yaml --dry-run

    helm install my-gateboard-discovery ./charts/gateboard-discovery --values charts/gateboard-discovery/values.yaml --dry-run

# Install

    helm install my-gateboard ./charts/gateboard --values charts/gateboard/values.yaml

    helm install my-gateboard-discovery ./charts/gateboard-discovery --values charts/gateboard-discovery/values.yaml

    helm list -A
