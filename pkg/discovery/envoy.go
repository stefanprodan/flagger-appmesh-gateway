package discovery

import (
	"fmt"
	"strconv"
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
	portName  string
}

type Upstream struct {
	Name    string
	Host    string
	Port    uint32
	Domain  string
	Prefix  string
	Retries uint32
	Timeout time.Duration
	Canary  *Canary
}

type Canary struct {
	PrimaryCluster string
	CanaryCluster  string
	CanaryWeight   int
}

func NewEnvoyConfig(cache cache.SnapshotCache, portName string) *EnvoyConfig {
	return &EnvoyConfig{
		version:   0,
		cache:     cache,
		upstreams: new(sync.Map),
		portName:  portName,
	}
}

func (e *EnvoyConfig) Delete(upstream string) {
	e.upstreams.Delete(upstream)
}

func (e *EnvoyConfig) Upsert(upstream string, service corev1.Service) {
	if e.shouldSync(service) {
		e.upstreams.Store(upstream, service)
	}
}

func (e *EnvoyConfig) Sync() {
	atomic.AddInt32(&e.version, 1)
	nodeId := e.cache.GetStatusKeys()[0]
	var listeners []cache.Resource
	var clusters []cache.Resource
	var vhosts []*route.VirtualHost
	domains := make(map[string]Upstream)
	e.upstreams.Range(func(key interface{}, value interface{}) bool {
		item := key.(string)
		service := value.(corev1.Service)
		ok, upstream := serviceToUpstream(service)
		if !ok {
			klog.Infof("service %s excluded, due to annotation", item)
			return true
		}

		cluster, port := serviceToCluster(service, e.portName, 5000)
		if cluster != nil {
			upstream.Name = cluster.Name
			upstream.Port = port
			clusters = append(clusters, cluster)
			domains[cluster.Name] = upstream
		} else {
			klog.Infof("service %s excluded, no port named '%s' found", item, e.portName)
		}
		return true
	})

	for cluster, up := range domains {
		vh := makeVirtualHost(cluster, up)
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

func (e *EnvoyConfig) shouldSync(svc corev1.Service) bool {
	var port int32
	for _, p := range svc.Spec.Ports {
		if p.Name == e.portName {
			port = p.Port
		}
	}
	if port == 0 {
		return false
	}
	exposeAn := "envoy.gateway.kubernetes.io/expose"
	for key, value := range svc.Annotations {
		if key == exposeAn && value == "false" {
			return false
		}
	}
	return true
}

func serviceToCluster(svc corev1.Service, portName string, timeout int) (cluster *ev2.Cluster, port uint32) {
	for _, p := range svc.Spec.Ports {
		if p.Name == portName {
			port = uint32(p.Port)
		}
	}
	if port == 0 {
		return
	}
	name := fmt.Sprintf("%s-%s-%d", svc.Name, svc.Namespace, port)
	address := fmt.Sprintf("%s.%s", svc.Name, svc.Namespace)
	cluster = makeCluster(name, address, port, timeout)
	return
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

func makeVirtualHost(name string, upstream Upstream) route.VirtualHost {
	action := &route.RouteAction{
		HostRewriteSpecifier: &route.RouteAction_HostRewrite{
			HostRewrite: upstream.Host,
		},
		ClusterSpecifier: &route.RouteAction_Cluster{
			Cluster: name,
		},
		Timeout: ptypes.DurationProto(upstream.Timeout),
	}

	if upstream.Canary != nil && upstream.Canary.CanaryCluster != "" && upstream.Canary.PrimaryCluster != "" {
		action = &route.RouteAction{
			HostRewriteSpecifier: &route.RouteAction_HostRewrite{
				HostRewrite: upstream.Host,
			},
			ClusterSpecifier: &route.RouteAction_WeightedClusters{
				WeightedClusters: &route.WeightedCluster{
					Clusters: []*route.WeightedCluster_ClusterWeight{
						{
							Name:   upstream.Canary.CanaryCluster,
							Weight: &wrappers.UInt32Value{Value: uint32(upstream.Canary.CanaryWeight)},
						},
						{
							Name:   upstream.Canary.PrimaryCluster,
							Weight: &wrappers.UInt32Value{Value: uint32(100 - upstream.Canary.CanaryWeight)},
						},
					},
				},
			},
			Timeout: ptypes.DurationProto(upstream.Timeout),
		}
	}

	r := &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: upstream.Prefix,
			},
		},
		Action: &route.Route_Route{
			Route: action,
		},
	}

	return route.VirtualHost{
		Name:        name,
		Domains:     []string{upstream.Domain},
		Routes:      []*route.Route{r},
		RetryPolicy: makeRetryPolicy(upstream.Retries),
		RequestHeadersToAdd: []*ecore.HeaderValueOption{
			{
				Header: &ecore.HeaderValue{
					Key:   "l5d-dst-override",
					Value: fmt.Sprintf("%s.svc.cluster.local:%d", upstream.Host, upstream.Port),
				},
			},
		},
		RequestHeadersToRemove: []string{"l5d-remote-ip", "l5d-server-id"},
	}
}

func makeRetryPolicy(retries uint32) *route.RetryPolicy {
	return &route.RetryPolicy{
		RetryOn:       "gateway-error,connect-failure,refused-stream",
		PerTryTimeout: ptypes.DurationProto(5 * time.Second),
		NumRetries:    &wrappers.UInt32Value{Value: retries},
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

func serviceToUpstream(svc corev1.Service) (bool, Upstream) {
	expose := true
	up := Upstream{
		Domain:  fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
		Host:    fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
		Prefix:  "/",
		Retries: 1,
		Timeout: 15 * time.Second,
		Canary:  &Canary{},
	}

	exposeAn := "envoy.gateway.kubernetes.io/expose"
	domainAn := "envoy.gateway.kubernetes.io/domain"
	timeoutAn := "envoy.gateway.kubernetes.io/timeout"
	retriesAn := "envoy.gateway.kubernetes.io/retries"
	primaryAn := "envoy.gateway.kubernetes.io/primary"
	canaryAn := "envoy.gateway.kubernetes.io/canary"
	canaryWeightAn := "envoy.gateway.kubernetes.io/canary-weight"
	for key, value := range svc.Annotations {
		if key == exposeAn && value == "false" {
			expose = false
		}
		if key == domainAn {
			up.Domain = value
		}
		if key == timeoutAn {
			d, err := time.ParseDuration(value)
			if err == nil {
				up.Timeout = d
			}
		}
		if key == retriesAn {
			r, err := strconv.Atoi(value)
			if err == nil {
				up.Retries = uint32(r)
			}
		}
		if key == primaryAn {
			up.Canary.PrimaryCluster = value
		}
		if key == canaryAn {
			up.Canary.CanaryCluster = value
		}
		if key == canaryWeightAn {
			r, err := strconv.Atoi(value)
			if err == nil {
				up.Canary.CanaryWeight = r
			}
		}
	}
	return expose, up
}
