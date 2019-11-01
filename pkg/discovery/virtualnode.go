package discovery

import (
	"fmt"

	appmeshv1 "github.com/aws/aws-app-mesh-controller-for-k8s/pkg/apis/appmesh/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
)

// VirtualNodeManager reconciles the gateway virtual node backend
type VirtualNodeManager struct {
	client           dynamic.Interface
	gatewayMesh      string
	gatewayName      string
	gatewayNamespace string
}

// NewVirtualNodeManager creates an App Mesh virtual node manager
func NewVirtualNodeManager(client dynamic.Interface, gatewayMesh string, gatewayName string, gatewayNamespace string) *VirtualNodeManager {
	return &VirtualNodeManager{
		client:           client,
		gatewayMesh:      gatewayMesh,
		gatewayName:      gatewayName,
		gatewayNamespace: gatewayNamespace,
	}
}

// Reconcile creates or updates the virtual node and its backends
func (vtm *VirtualNodeManager) Reconcile(backends []string) error {
	vnName := vtm.gatewayName
	var vnBackends []appmeshv1.Backend
	for _, value := range backends {
		vnBackends = append(vnBackends, appmeshv1.Backend{
			VirtualService: appmeshv1.VirtualServiceBackend{VirtualServiceName: value},
		})
	}
	spec := appmeshv1.VirtualNodeSpec{
		MeshName: vtm.gatewayMesh,
		Listeners: []appmeshv1.Listener{
			{PortMapping: appmeshv1.PortMapping{
				Port:     444,
				Protocol: "http",
			}},
		},
		ServiceDiscovery: &appmeshv1.ServiceDiscovery{
			Dns: &appmeshv1.DnsServiceDiscovery{
				HostName: fmt.Sprintf("%s.%s", vtm.gatewayName, vtm.gatewayNamespace),
			}},
		Backends: vnBackends,
	}

	vn := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "VirtualNode",
			"apiVersion": "appmesh.k8s.aws/v1beta1",
			"metadata": map[string]interface{}{
				"name": vnName,
			},
			"spec": spec,
		},
	}

	client := vtm.client.Resource(schema.GroupVersionResource{
		Group:    "appmesh.k8s.aws",
		Version:  "v1beta1",
		Resource: "virtualnodes",
	})

	_, err := client.Namespace(vtm.gatewayNamespace).Get(vnName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, createErr := client.Namespace(vtm.gatewayNamespace).Create(vn, metav1.CreateOptions{})
		if createErr != nil && !errors.IsNotFound(createErr) {
			return fmt.Errorf("failed to create gateway virtual node: %v", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get gateway virtual node: %v", err)
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		gw, err := client.Namespace(vtm.gatewayNamespace).Get(vnName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		vn = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "VirtualNode",
				"apiVersion": "appmesh.k8s.aws/v1beta1",
				"metadata": map[string]interface{}{
					"name":            vnName,
					"resourceVersion": gw.GetResourceVersion(),
				},
				"spec": spec,
			},
		}
		_, err = client.Namespace(vtm.gatewayNamespace).Update(vn, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	})
	if retryErr != nil {
		return fmt.Errorf("failed to update gateway virtual node: %v", retryErr)
	}

	return nil
}
