package envoy

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"k8s.io/klog"
)

type Snapshot struct {
	version   uint64
	cache     cache.SnapshotCache
	upstreams *sync.Map
}

func NewSnapshot(cache cache.SnapshotCache) *Snapshot {
	return &Snapshot{
		version:   0,
		cache:     cache,
		upstreams: new(sync.Map),
	}
}

func (s *Snapshot) Store(key string, value Upstream) {
	s.upstreams.Store(key, value)
}

func (s *Snapshot) Delete(key string) {
	s.upstreams.Delete(key)
}

func (s *Snapshot) Len() int {
	var length int
	s.upstreams.Range(func(_, _ interface{}) bool {
		length++
		return true
	})
	return length
}

func (s *Snapshot) Sync() {
	if len(s.cache.GetStatusKeys()) < 1 {
		klog.Errorf("cache has no node IDs")
		return
	}
	nodeId := s.cache.GetStatusKeys()[0]

	var listeners []cache.Resource
	var clusters []cache.Resource
	var vhosts []*route.VirtualHost

	s.upstreams.Range(func(key interface{}, value interface{}) bool {
		upstream := value.(Upstream)
		cluster := newCluster(upstream, time.Second)
		clusters = append(clusters, cluster)
		vh := newVirtualHost(upstream)
		vhosts = append(vhosts, &vh)
		return true
	})

	cm := newConnectionManager("local_route", vhosts, 5*time.Second)
	httpListener, err := newListener("listener_http", "0.0.0.0", 8080, cm)
	listeners = append(listeners, httpListener)

	atomic.AddUint64(&s.version, 1)
	snapshot := cache.NewSnapshot(fmt.Sprint(s.version), nil, clusters, nil, listeners)
	err = s.cache.SetSnapshot(nodeId, snapshot)
	if err != nil {
		klog.Errorf("error while setting snapshot %v", err)
	}
}
