package discovery

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	appmeshv1 "github.com/aws/aws-app-mesh-controller-for-k8s/pkg/apis/appmesh/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/stefanprodan/appmesh-gateway/pkg/envoy"
)

// VirtualServiceManager transforms virtual service to upstreams
type VirtualServiceManager struct {
	client dynamic.Interface
	optIn  bool
}

// NewVirtualServiceManager creates an App Mesh virtual service manager
func NewVirtualServiceManager(client dynamic.Interface, optIn bool) *VirtualServiceManager {
	return &VirtualServiceManager{
		client: client,
		optIn:  optIn,
	}
}

// ConvertToUpstream converts the App Mesh virtual service to an Upstream
func (vsm *VirtualServiceManager) ConvertToUpstream(vs appmeshv1.VirtualService) envoy.Upstream {
	port := uint32(80)
	for _, value := range vs.Spec.VirtualRouter.Listeners {
		port = uint32(value.PortMapping.Port)
	}

	up := envoy.Upstream{
		Name: fmt.Sprintf("%s-%d", vs.Name, port),
		Domains: []string{
			vs.Name,
			fmt.Sprintf("%s:%d", vs.Name, port),
		},
		Port:    port,
		Host:    vs.Name,
		Prefix:  "/",
		Retries: 2,
		Timeout: 45 * time.Second,
	}

	appendDomain := func(slice []string, i string) []string {
		for _, ele := range slice {
			if ele == i {
				return slice
			}
		}
		return append(slice, i)
	}

	for key, value := range vs.Annotations {
		if key == envoy.GatewayDomain {
			up.Domains = appendDomain(up.Domains, value)
		}
		if key == envoy.GatewayTimeout {
			d, err := time.ParseDuration(value)
			if err == nil {
				up.Timeout = d
			}
		}
		if key == envoy.GatewayRetries {
			r, err := strconv.Atoi(value)
			if err == nil {
				up.Retries = uint32(r)
			}
		}
	}
	return up
}

// IsValid checks if a virtual service service is eligible
func (vsm *VirtualServiceManager) IsValid(vs appmeshv1.VirtualService) bool {
	if vs.Spec.VirtualRouter == nil ||
		len(vs.Spec.VirtualRouter.Listeners) < 1 ||
		vs.Spec.VirtualRouter.Listeners[0].PortMapping.Port < 1 {
		return false
	}

	for key, value := range vs.Annotations {
		if vsm.optIn && key == envoy.GatewayExpose && value != "true" {
			return false
		}
		if key == envoy.GatewayExpose && value == "false" {
			return false
		}
	}
	return true
}

func (vsm *VirtualServiceManager) VirtualServiceFromUnstructured(obj *unstructured.Unstructured) (*appmeshv1.VirtualService, error) {
	b, _ := json.Marshal(&obj)
	var svc appmeshv1.VirtualService
	err := json.Unmarshal(b, &svc)
	if err != nil {
		return nil, err
	}

	return &svc, nil
}
