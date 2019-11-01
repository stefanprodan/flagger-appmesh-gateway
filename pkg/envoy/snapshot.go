package envoy

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/mitchellh/hashstructure"
	"k8s.io/klog"
)

// Snapshot manages Envoy clusters and listeners cache snapshots
type Snapshot struct {
	version   uint64
	cache     cache.SnapshotCache
	upstreams *sync.Map
	checksum  uint64
	nodeId    string
}

// NewSnapshot creates an Envoy cache snapshot manager
func NewSnapshot(cache cache.SnapshotCache) *Snapshot {
	return &Snapshot{
		version:   0,
		cache:     cache,
		upstreams: new(sync.Map),
	}
}

// Store inserts or updates an upstream in the in-memory cache
func (s *Snapshot) Store(key string, value Upstream) {
	s.upstreams.Store(key, value)
}

// Delete removes an upstream from the in-memory cache
func (s *Snapshot) Delete(key string) {
	s.upstreams.Delete(key)
}

// Len returns the number of upstreams stored in the in-memory cache
func (s *Snapshot) Len() int {
	var length int
	s.upstreams.Range(func(_, _ interface{}) bool {
		length++
		return true
	})
	return length
}

func (s *Snapshot) getNodeId() (string, error) {
	if s.nodeId != "" {
		return s.nodeId, nil
	}
	if len(s.cache.GetStatusKeys()) < 1 {
		return "", fmt.Errorf("cache has no node IDs, status keys %d", len(s.cache.GetStatusKeys()))
	}
	return s.cache.GetStatusKeys()[0], nil
}

// Sync reconciles the in-memory cache of upstreams
// with the Envoy cache by creating a new snapshot
func (s *Snapshot) Sync() error {
	nodeId, err := s.getNodeId()
	if err != nil {
		return err
	}
	upstreams := make(map[string]Upstream)
	var listeners []cache.Resource
	var clusters []cache.Resource
	var vhosts []*route.VirtualHost

	s.upstreams.Range(func(key interface{}, value interface{}) bool {
		k := key.(string)
		upstream := value.(Upstream)
		upstreams[k] = upstream
		return true
	})

	checksum, err := hashstructure.Hash(upstreams, nil)
	if err != nil {
		return fmt.Errorf("checksum error %v", err)
	}

	if checksum == s.checksum {
		return nil
	}

	for _, upstream := range upstreams {
		cluster := newCluster(upstream, time.Second)
		clusters = append(clusters, cluster)
		vh := newVirtualHost(upstream)
		vhosts = append(vhosts, &vh)
	}

	cm := newConnectionManager("local_route", vhosts, 5*time.Second)
	httpListener, err := newListener("listener_http", "0.0.0.0", 8080, cm)
	listeners = append(listeners, httpListener)

	atomic.AddUint64(&s.version, 1)
	snapshot := cache.NewSnapshot(fmt.Sprint(s.version), nil, clusters, nil, listeners)

	if err := snapshot.Consistent(); err != nil {
		return err
	}

	err = s.cache.SetSnapshot(nodeId, snapshot)
	if err != nil {
		return fmt.Errorf("error while setting snapshot %v", err)
	}

	atomic.StoreUint64(&s.checksum, checksum)
	klog.Infof("cache updated for %d services, version %d, checksum %d", len(upstreams), s.version, checksum)

	return nil
}
