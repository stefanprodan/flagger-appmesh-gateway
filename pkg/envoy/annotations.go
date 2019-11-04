package envoy

import "strconv"

const (
	// GatewayPrefix prefix annotation
	GatewayPrefix = "gateway.appmesh.k8s.aws/"
	// GatewayExpose expose boolean annotation
	GatewayExpose = GatewayPrefix + "expose"
	// GatewayDomain annotation with a comma separated list of public or internal domains
	GatewayDomain = GatewayPrefix + "domain"
	// GatewayTimeout max response duration annotation
	GatewayTimeout = GatewayPrefix + "timeout"
	// GatewayRetries number of retries annotation
	GatewayRetries = GatewayPrefix + "retries"
	// GatewayPrimary primary virtual service name annotation
	GatewayPrimary = GatewayPrefix + "primary"
	// GatewayCanary canary virtual service name annotation
	GatewayCanary = GatewayPrefix + "canary"
	// GatewayCanaryWeight traffic weight percentage annotation
	GatewayCanaryWeight = GatewayPrefix + "canary-weight"
)

// CanaryFromAnnotations parses the annotations and returns a canary object
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
