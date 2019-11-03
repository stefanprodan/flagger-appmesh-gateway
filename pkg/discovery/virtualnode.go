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
	"k8s.io/klog"
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
func (vnm *VirtualNodeManager) Reconcile(backends []string) error {
	vnName := vnm.gatewayName
	var vnBackends []appmeshv1.Backend
	for _, value := range backends {
		vnBackends = append(vnBackends, appmeshv1.Backend{
			VirtualService: appmeshv1.VirtualServiceBackend{VirtualServiceName: value},
		})
	}
	spec := appmeshv1.VirtualNodeSpec{
		MeshName: vnm.gatewayMesh,
		Listeners: []appmeshv1.Listener{
			{PortMapping: appmeshv1.PortMapping{
				Port:     444,
				Protocol: "http",
			}},
		},
		ServiceDiscovery: &appmeshv1.ServiceDiscovery{
			Dns: &appmeshv1.DnsServiceDiscovery{
				HostName: fmt.Sprintf("%s.%s", vnm.gatewayName, vnm.gatewayNamespace),
			}},
		Backends: vnBackends,
	}

	vn := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "VirtualNode",
			"apiVersion": appmeshv1.SchemeGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name": vnName,
			},
			"spec": spec,
		},
	}

	client := vnm.client.Resource(schema.GroupVersionResource{
		Group:    "appmesh.k8s.aws",
		Version:  "v1beta1",
		Resource: "virtualnodes",
	})

	_, err := client.Namespace(vnm.gatewayNamespace).Get(vnName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, createErr := client.Namespace(vnm.gatewayNamespace).Create(vn, metav1.CreateOptions{})
		if createErr != nil && !errors.IsNotFound(createErr) {
			return fmt.Errorf("failed to create gateway virtual node: %v", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get gateway virtual node: %v", err)
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		gw, err := client.Namespace(vnm.gatewayNamespace).Get(vnName, metav1.GetOptions{})
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
		_, err = client.Namespace(vnm.gatewayNamespace).Update(vn, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	})
	if retryErr != nil {
		return fmt.Errorf("failed to update gateway virtual node: %v", retryErr)
	}

	klog.Infof("virtual node %s updated with %d backends", vnName, len(backends))

	return nil
}

// CheckAccess verifies if RBAC is allowing virtual nodes read operations
func (vnm *VirtualNodeManager) CheckAccess() error {
	vnr, _ := schema.ParseResourceArg("virtualnodes.v1beta1.appmesh.k8s.aws")
	_, err := vnm.client.Resource(*vnr).Namespace(vnm.gatewayNamespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	return nil
}
