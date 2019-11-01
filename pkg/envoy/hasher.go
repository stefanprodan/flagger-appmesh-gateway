package envoy

import envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

// Hasher computes string identifiers for Envoy nodes.
type Hasher struct{}

// ID returns the Envoy node ID
func (h Hasher) ID(node *envoycore.Node) string {
	if node == nil {
		return "envoy"
	}
	return node.Id
}
