package discovery

import (
	envoy "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"k8s.io/klog"
)

func NewCache() cache.SnapshotCache {
	return cache.NewSnapshotCache(true, Hasher{}, cacheLog{})
}

type Hasher struct {
}

func (h Hasher) ID(node *envoy.Node) string {
	if node == nil {
		return "envoy"
	}
	return node.Id
}

type cacheLog struct{}

func (l cacheLog) Infof(format string, args ...interface{}) {
	klog.Infof(format, args...)
}

func (l cacheLog) Errorf(format string, args ...interface{}) {
	klog.Errorf(format, args...)
}
