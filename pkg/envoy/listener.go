package envoy

import (
	"time"

	envoyv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func newListener(name, address string, port uint32, cm *hcm.HttpConnectionManager) (*envoyv2.Listener, error) {
	cmAny, err := ptypes.MarshalAny(cm)
	if err != nil {
		return nil, err
	}

	return &envoyv2.Listener{
		Name:    name,
		Address: newAddress(address, port),
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

func newConnectionManager(routeName string, vhosts []*route.VirtualHost, drainTimeout time.Duration) *hcm.HttpConnectionManager {
	return &hcm.HttpConnectionManager{
		CodecType:    hcm.HttpConnectionManager_AUTO,
		DrainTimeout: ptypes.DurationProto(drainTimeout),
		StatPrefix:   "ingress_http",
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoyv2.RouteConfiguration{
				Name:             routeName,
				VirtualHosts:     vhosts,
				ValidateClusters: &wrappers.BoolValue{Value: true},
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
		UseRemoteAddress: &wrappers.BoolValue{Value: true},
	}
}
