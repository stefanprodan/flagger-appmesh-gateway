package envoy

import envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

type Hasher struct{}

func (h Hasher) ID(node *envoycore.Node) string {
	if node == nil {
		return "envoy"
	}
	return node.Id
}
