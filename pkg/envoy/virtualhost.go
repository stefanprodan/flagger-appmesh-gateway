package envoy

import (
	"fmt"
	"time"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func newVirtualHost(upstream Upstream) route.VirtualHost {
	action := &route.RouteAction{
		HostRewriteSpecifier: &route.RouteAction_HostRewrite{
			HostRewrite: upstream.Host,
		},
		ClusterSpecifier: &route.RouteAction_Cluster{
			Cluster: upstream.Name,
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
		Name:        upstream.Name,
		Domains:     upstream.Domains,
		Routes:      []*route.Route{r},
		RetryPolicy: makeRetryPolicy(upstream.Retries, upstream.Timeout),
		RequestHeadersToAdd: []*envoycore.HeaderValueOption{
			{
				Header: &envoycore.HeaderValue{
					Key:   "l5d-dst-override",
					Value: fmt.Sprintf("%s.svc.cluster.local:%d", upstream.Host, upstream.Port),
				},
			},
		},
		RequestHeadersToRemove: []string{"l5d-remote-ip", "l5d-server-id"},
	}
}

func makeRetryPolicy(retries uint32, timeout time.Duration) *route.RetryPolicy {
	return &route.RetryPolicy{
		RetryOn:                       "connect-failure,refused-stream,unavailable,cancelled,resource-exhausted,retriable-status-codes",
		PerTryTimeout:                 ptypes.DurationProto(timeout),
		NumRetries:                    &wrappers.UInt32Value{Value: retries},
		HostSelectionRetryMaxAttempts: 5,
		RetriableStatusCodes:          []uint32{503},
		RetryHostPredicate: []*route.RetryPolicy_RetryHostPredicate{
			{Name: "envoy.retry_host_predicates.previous_hosts"},
		},
	}
}
