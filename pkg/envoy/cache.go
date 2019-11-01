package envoy

import (
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"k8s.io/klog"
)

// NewCache creates an Envoy cache
func NewCache(ads bool) cache.SnapshotCache {
	return cache.NewSnapshotCache(ads, Hasher{}, log{})
}

type log struct{}

func (l log) Infof(format string, args ...interface{}) {
	klog.V(4).Infof(format, args...)
}

func (l log) Errorf(format string, args ...interface{}) {
	klog.Errorf(format, args...)
}
