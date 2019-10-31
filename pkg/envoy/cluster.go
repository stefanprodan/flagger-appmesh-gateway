package envoy

import (
	"time"

	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func newCluster(upstream Upstream, timeout time.Duration) *envoyv2.Cluster {
	return &envoyv2.Cluster{
		Name:                 upstream.Name,
		ConnectTimeout:       ptypes.DurationProto(timeout),
		ClusterDiscoveryType: &envoyv2.Cluster_Type{Type: envoyv2.Cluster_STRICT_DNS},
		DnsLookupFamily:      envoyv2.Cluster_V4_ONLY,
		LbPolicy:             envoyv2.Cluster_LEAST_REQUEST,
		LoadAssignment: &envoyv2.ClusterLoadAssignment{
			ClusterName: upstream.Name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: []*endpoint.LbEndpoint{{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: newAddress(upstream.Host, upstream.Port),
						},
					},
				}},
			}},
		},
		CircuitBreakers: &cluster.CircuitBreakers{
			Thresholds: []*cluster.CircuitBreakers_Thresholds{{
				MaxRetries: &wrappers.UInt32Value{Value: uint32(1024)},
			}},
		},
	}
}

func newAddress(address string, port uint32) *envoycore.Address {
	return &envoycore.Address{Address: &envoycore.Address_SocketAddress{
		SocketAddress: &envoycore.SocketAddress{
			Address:  address,
			Protocol: envoycore.SocketAddress_TCP,
			PortSpecifier: &envoycore.SocketAddress_PortValue{
				PortValue: port,
			},
		},
	}}
}
