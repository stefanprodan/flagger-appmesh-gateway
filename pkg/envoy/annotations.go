package envoy

import "strconv"

const (
	GatewayPrefix       = "gateway.appmesh.k8s.aws/"
	GatewayExpose       = GatewayPrefix + "expose"
	GatewayDomain       = GatewayPrefix + "domain"
	GatewayTimeout      = GatewayPrefix + "timeout"
	GatewayRetries      = GatewayPrefix + "retries"
	GatewayPrimary      = GatewayPrefix + "primary"
	GatewayCanary       = GatewayPrefix + "canary"
	GatewayCanaryWeight = GatewayPrefix + "canary-weight"
)

func CanaryFromAnnotations(an map[string]string) *Canary {
	var primaryCluster string
	var canaryCluster string
	var canaryWeight int
	for key, value := range an {
		if key == GatewayPrimary {
			primaryCluster = value
		}
		if key == GatewayCanary {
			canaryCluster = value
		}
		if key == GatewayCanaryWeight {
			r, err := strconv.Atoi(value)
			if err == nil {
				canaryWeight = r
			}
		}
	}

	if primaryCluster != "" && canaryCluster != "" {
		return &Canary{
			PrimaryCluster: primaryCluster,
			CanaryCluster:  canaryCluster,
			CanaryWeight:   canaryWeight,
		}
	}

	return nil
}
