package discovery

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ev2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	ecore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

type EnvoyConfig struct {
	version   int32
	cache     cache.SnapshotCache
	upstreams *sync.Map
}

func NewEnvoyConfig(cache cache.SnapshotCache) *EnvoyConfig {
	return &EnvoyConfig{
		version:   0,
		cache:     cache,
		upstreams: new(sync.Map),
	}
}

func (e *EnvoyConfig) Delete(upstream string) {
	e.upstreams.Delete(upstream)
}

func (e *EnvoyConfig) Upsert(upstream string, service corev1.Service) {
	e.upstreams.Store(upstream, service)
}

func (e *EnvoyConfig) Sync() {
	atomic.AddInt32(&e.version, 1)
	nodeId := e.cache.GetStatusKeys()[0]
	var listeners []cache.Resource
	var clusters []cache.Resource
	var vhosts []*route.VirtualHost
	domains := make(map[string]string)
	e.upstreams.Range(func(key interface{}, value interface{}) bool {
		item := key.(string)
		service := value.(corev1.Service)
		portName := "http"
		timeout := 25000
		cluster := serviceToCluster(service, portName, timeout)
		if cluster != nil {
			clusters = append(clusters, cluster)
			domains[cluster.Name] = fmt.Sprintf("%s.%s", service.Name, service.Namespace)
		} else {
			klog.Infof("service %s excluded, no port named '%s' found", item, portName)
		}
		return true
	})

	for cluster, domain := range domains {
		vh := makeVirtualHost(cluster, domain, "/", cluster, uint32(1))
		vhosts = append(vhosts, &vh)
	}

	cm := makeConnectionManager("local_route", vhosts, 10)
	httpListener, err := makeListener("listener_http", "0.0.0.0", 8080, cm)
	listeners = append(listeners, httpListener)

	snap := cache.NewSnapshot(fmt.Sprint(e.version), nil, clusters, nil, listeners)
	err = e.cache.SetSnapshot(nodeId, snap)
	if err != nil {
		klog.Errorf("error while setting snapshot %v", err)
	}
}

func serviceToCluster(svc corev1.Service, portName string, timeout int) *ev2.Cluster {
	var port uint32
	for _, p := range svc.Spec.Ports {
		if p.Name == portName {
			port = uint32(p.Port)
		}
	}
	if port == 0 {
		return nil
	}
	name := fmt.Sprintf("%s-%s-%d", svc.Name, svc.Namespace, port)
	address := fmt.Sprintf("%s.%s", svc.Name, svc.Namespace)
	return makeCluster(name, address, port, timeout)
}

func makeCluster(name string, address string, port uint32, timeout int) *ev2.Cluster {
	return &ev2.Cluster{
		Name:                 name,
		ConnectTimeout:       ptypes.DurationProto(time.Duration(timeout) * time.Millisecond),
		ClusterDiscoveryType: &ev2.Cluster_Type{Type: ev2.Cluster_STRICT_DNS},
		DnsLookupFamily:      ev2.Cluster_V4_ONLY,
		LbPolicy:             ev2.Cluster_LEAST_REQUEST,
		LoadAssignment: &ev2.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: makeAddress(address, port),
						},
					},
				}},
			}},
		},
	}
}

func makeAddress(address string, port uint32) *ecore.Address {
	return &ecore.Address{Address: &ecore.Address_SocketAddress{
		SocketAddress: &ecore.SocketAddress{
			Address:  address,
			Protocol: ecore.SocketAddress_TCP,
			PortSpecifier: &ecore.SocketAddress_PortValue{
				PortValue: port,
			},
		},
	}}
}

func makeVirtualHost(name string, domain string, prefix string, clusterName string, retries uint32) route.VirtualHost {
	r := &route.Route{
		Match:  makeRouteMatch(prefix),
		Action: makeRouteAction(domain, clusterName),
	}
	return route.VirtualHost{
		Name:        name,
		Domains:     []string{domain},
		Routes:      []*route.Route{r},
		RetryPolicy: makeRetryPolicy(retries),
	}
}

func makeRetryPolicy(retries uint32) *route.RetryPolicy {
	return &route.RetryPolicy{
		RetryOn:       "gateway-error,connect-failure,refused-stream",
		PerTryTimeout: ptypes.DurationProto(5 * time.Second),
		NumRetries:    &wrappers.UInt32Value{Value: retries},
	}
}

func makeRouteMatch(prefix string) *route.RouteMatch {
	return &route.RouteMatch{
		PathSpecifier: &route.RouteMatch_Prefix{
			Prefix: prefix,
		},
	}
}

func makeRouteAction(domain string, cluster string) *route.Route_Route {
	return &route.Route_Route{
		Route: &route.RouteAction{
			HostRewriteSpecifier: &route.RouteAction_HostRewrite{
				HostRewrite: domain,
			},
			ClusterSpecifier: &route.RouteAction_Cluster{
				Cluster: cluster,
			},
		},
	}
}

func makeConnectionManager(routeName string, vhosts []*route.VirtualHost, drainTimeout int) *hcm.HttpConnectionManager {
	return &hcm.HttpConnectionManager{
		CodecType:    hcm.HttpConnectionManager_AUTO,
		DrainTimeout: ptypes.DurationProto(time.Duration(drainTimeout) * time.Second),
		StatPrefix:   "ingress_http",
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: &ev2.RouteConfiguration{
				Name:         routeName,
				VirtualHosts: vhosts,
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
	}
}

func makeListener(name, address string, port uint32, cm *hcm.HttpConnectionManager) (*ev2.Listener, error) {
	cmAny, err := ptypes.MarshalAny(cm)
	if err != nil {
		return nil, err
	}
	return &ev2.Listener{
		Name:    name,
		Address: makeAddress(address, port),
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: cmAny,
				},
			}},
		}},
	}, nil
}
