# Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

    helm repo add miniapi https://udhos.github.io/miniapi

Update files from repo:

    helm repo update

Search miniapi:

    helm search repo miniapi -l
    NAME           	CHART VERSION	APP VERSION	DESCRIPTION
    miniapi/miniapi	0.1.7        	0.0.2      	A Helm chart for miniapi
    miniapi/miniapi	0.1.6        	0.0.2      	A Helm chart for miniapi
    miniapi/miniapi	0.1.5        	0.0.1      	A Helm chart for miniapi
    miniapi/miniapi	0.1.4        	0.0.1      	A Helm chart for miniapi
    miniapi/miniapi	0.1.3        	0.0.1      	A Helm chart for miniapi

To install the miniapi chart:

    helm install my-miniapi miniapi/miniapi
    #            ^          ^       ^
    #            |          |        \__ chart
    #            |          |
    #            |           \__________ repo
    #            |
    #             \_____________________ release (chart instance installed in cluster)

To uninstall the chart:

    helm uninstall my-miniapi

# Source

<https://github.com/udhos/miniapi>
