# Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

    helm repo add gateboard https://udhos.github.io/gateboard

Update files from repo:

    helm repo update

Search gateboard:

    helm search repo bateboard -l
    NAME           	                CHART VERSION	APP VERSION	DESCRIPTION
    gateboard/gateboard	            0.0.0        	0.0.8      	A Helm chart for gateboard
    bateboard/gateboard-discovery	0.0.0        	0.0.11     	A Helm chart for gateboard-discovery

To install the chart:

    helm install my-gateboard gateboard/gateboard
    helm install my-discovery gateboard/gateboard-discovery
    #            ^            ^         ^
    #            |            |          \__ chart
    #            |            |
    #            |             \____________ repo
    #            |
    #             \_________________________ release (chart instance installed in cluster)

To uninstall the chart:

    helm uninstall my-discovery

# Source

<https://github.com/udhos/gateboard>
