package server

import (
	"context"
	"sync"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"k8s.io/klog"
)

type callbacks struct {
	signal   chan struct{}
	fetches  int
	requests int
	mu       sync.Mutex
}

func (cb *callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	klog.V(4).Infof("report requests %v", cb.requests)
}

func (cb *callbacks) OnStreamOpen(_ context.Context, id int64, typ string) error {
	klog.V(4).Infof("stream %d open for %s", id, typ)
	return nil
}

func (cb *callbacks) OnStreamClosed(id int64) {
	klog.V(4).Infof("stream %d closed", id)
}

func (cb *callbacks) OnStreamRequest(int64, *v2.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.requests++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}

func (cb *callbacks) OnStreamResponse(int64, *v2.DiscoveryRequest, *v2.DiscoveryResponse) {
	cb.Report()
}

func (cb *callbacks) OnFetchRequest(_ context.Context, req *v2.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.fetches++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}

func (cb *callbacks) OnFetchResponse(*v2.DiscoveryRequest, *v2.DiscoveryResponse) {}
