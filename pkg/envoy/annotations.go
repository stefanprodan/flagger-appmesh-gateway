package envoy

import "strconv"

const (
	AnPrefix       = "envoy.gateway.kubernetes.io/"
	AnExpose       = AnPrefix + "expose"
	AnDomain       = AnPrefix + "domain"
	AnTimeout      = AnPrefix + "timeout"
	AnRetries      = AnPrefix + "retries"
	AnPrimary      = AnPrefix + "primary"
	AnCanary       = AnPrefix + "canary"
	AnCanaryWeight = AnPrefix + "canary-weight"
)

func CanaryFromAnnotations(an map[string]string) *Canary {
	var primaryCluster string
	var canaryCluster string
	var canaryWeight int
	for key, value := range an {
		if key == AnPrimary {
			primaryCluster = value
		}
		if key == AnCanary {
			canaryCluster = value
		}
		if key == AnCanaryWeight {
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
