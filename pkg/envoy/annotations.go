package envoy

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
