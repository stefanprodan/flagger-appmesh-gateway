package discovery

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/stefanprodan/appmesh-gateway/pkg/envoy"
)

// AppmeshDiscovery watches Kubernetes for App Mesh virtual services and
// creates or deletes Envoy clusters and virtual hosts
type AppmeshDiscovery struct {
	client           dynamic.Interface
	indexer          cache.Indexer
	queue            workqueue.RateLimitingInterface
	informer         cache.Controller
	snapshot         *envoy.Snapshot
	optIn            bool
	gatewayMesh      string
	gatewayName      string
	gatewayNamespace string
}

// NewAppmeshDiscovery starts watching for App Mesh virtual services
func NewAppmeshDiscovery(client dynamic.Interface, namespace string, snapshot *envoy.Snapshot, optIn bool, gatewayMesh string, gatewayName string, gatewayNamespace string) *AppmeshDiscovery {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, 0, namespace, nil)
	gvr, _ := schema.ParseResourceArg("virtualservices.v1beta1.appmesh.k8s.aws")
	vsFactory := factory.ForResource(*gvr)
	informer := vsFactory.Informer()
	indexer := vsFactory.Informer().GetIndexer()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	}
	informer.AddEventHandler(handlers)

	return &AppmeshDiscovery{
		client:           client,
		informer:         informer,
		indexer:          indexer,
		queue:            queue,
		snapshot:         snapshot,
		optIn:            optIn,
		gatewayMesh:      gatewayMesh,
		gatewayName:      gatewayName,
		gatewayNamespace: gatewayNamespace,
	}
}

// Run starts the App Mesh discovery controller
func (ad *AppmeshDiscovery) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer ad.queue.ShutDown()

	go ad.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, ad.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	ad.syncAll()

	for i := 0; i < threadiness; i++ {
		go wait.Until(ad.runWorker, time.Second, stopCh)
	}

	tickChan := time.NewTicker(5 * time.Minute).C
	for {
		select {
		case <-tickChan:
			ad.syncAll()
		case <-stopCh:
			klog.Info("stopping Kubernetes discovery workers")
			return
		}
	}
}

func (ad *AppmeshDiscovery) sync(key string) error {
	_, exists, err := ad.indexer.GetByKey(key)
	if err != nil {
		klog.Errorf("fetching object with key %s from store failed %v", key, err)
		return err
	}

	if !exists {
		klog.Infof("deleting %s from cache", key)
		ad.snapshot.Delete(key)
	}

	ad.syncAll()
	return nil
}

func (ad *AppmeshDiscovery) syncAll() {
	var backends []string
	for _, value := range ad.indexer.List() {
		un := value.(*unstructured.Unstructured)
		vs, err := ad.toVirtualService(un)
		if err != nil {
			klog.Errorf("unmarshal object %s from store failed %v", un.GetName(), err)
			return
		}
		if ad.vsIsValid(*vs) {
			backends = append(backends, vs.Name)
			ad.snapshot.Store(fmt.Sprintf("%s/%s", vs.Namespace, vs.Name), ad.vsToUpstream(*vs))
		}
	}

	klog.Infof("updating gateway virtual node with %d backends", len(backends))
	err := ad.updateVirtualNode(backends)
	if err != nil {
		klog.Error(err)
		return
	}

	err = ad.snapshot.Sync()
	if err != nil {
		klog.Errorf("snapshot error %v", err)
		return
	}
	klog.Infof("cache updated for %d services", ad.snapshot.Len())

}

func (ad *AppmeshDiscovery) handleErr(err error, key interface{}) {
	if err == nil {
		ad.queue.Forget(key)
		return
	}

	if ad.queue.NumRequeues(key) < 5 {
		klog.Infof("error syncing %v: %v", key, err)
		ad.queue.AddRateLimited(key)
		return
	}

	ad.queue.Forget(key)
	runtime.HandleError(err)
	klog.Infof("dropping %q out of the queue: %v", key, err)
}

func (ad *AppmeshDiscovery) processNextItem() bool {
	key, quit := ad.queue.Get()
	if quit {
		return false
	}
	defer ad.queue.Done(key)

	err := ad.sync(key.(string))
	ad.handleErr(err, key)
	return true
}

func (ad *AppmeshDiscovery) runWorker() {
	for ad.processNextItem() {
	}
}

// vsToUpstream converts the App Mesh virtual service to an Upstream
func (ad *AppmeshDiscovery) vsToUpstream(vs VirtualService) envoy.Upstream {
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

// vsIsValid checks if a virtual service service is eligible
func (ad *AppmeshDiscovery) vsIsValid(vs VirtualService) bool {
	if vs.Spec.VirtualRouter == nil ||
		len(vs.Spec.VirtualRouter.Listeners) < 1 ||
		vs.Spec.VirtualRouter.Listeners[0].PortMapping.Port < 1 {
		return false
	}

	for key, value := range vs.Annotations {
		if ad.optIn && key == envoy.GatewayExpose && value != "true" {
			return false
		}
		if key == envoy.GatewayExpose && value == "false" {
			return false
		}
	}
	return true
}

func (ad *AppmeshDiscovery) toVirtualService(obj *unstructured.Unstructured) (*VirtualService, error) {
	b, _ := json.Marshal(&obj)
	var svc VirtualService
	err := json.Unmarshal(b, &svc)
	if err != nil {
		return nil, err
	}

	return &svc, nil
}

func (ad *AppmeshDiscovery) updateVirtualNode(backends []string) error {
	vnName := ad.gatewayName
	var vnBackends []Backend
	for _, value := range backends {
		vnBackends = append(vnBackends, Backend{
			VirtualService: VirtualServiceBackend{VirtualServiceName: value},
		})
	}
	spec := VirtualNodeSpec{
		MeshName: ad.gatewayMesh,
		Listeners: []Listener{
			{PortMapping: PortMapping{
				Port:     444,
				Protocol: "http",
			}},
		},
		ServiceDiscovery: &ServiceDiscovery{Dns: &DnsServiceDiscovery{
			HostName: fmt.Sprintf("%s.%s", ad.gatewayName, ad.gatewayNamespace),
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

	client := ad.client.Resource(schema.GroupVersionResource{
		Group:    "appmesh.k8s.aws",
		Version:  "v1beta1",
		Resource: "virtualnodes",
	})

	_, err := client.Namespace(ad.gatewayNamespace).Get(vnName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, createErr := client.Namespace(ad.gatewayNamespace).Create(vn, metav1.CreateOptions{})
		if createErr != nil && !errors.IsNotFound(createErr) {
			return fmt.Errorf("failed to create gateway virtual node: %v", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to get gateway virtual node: %v", err)
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		gw, err := client.Namespace(ad.gatewayNamespace).Get(vnName, metav1.GetOptions{})
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
		_, err = client.Namespace(ad.gatewayNamespace).Update(vn, metav1.UpdateOptions{})
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

type VirtualService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualServiceSpec `json:"spec,omitempty"`
}
type VirtualServiceSpec struct {
	VirtualRouter *VirtualRouter `json:"virtualRouter,omitempty"`
}
type VirtualRouter struct {
	Name      string                  `json:"name"`
	Listeners []VirtualRouterListener `json:"listeners,omitempty"`
}
type VirtualRouterListener struct {
	PortMapping PortMapping `json:"portMapping"`
}
type PortMapping struct {
	Port     int64  `json:"port"`
	Protocol string `json:"protocol"`
}
type VirtualNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualNodeSpec `json:"spec,omitempty"`
}
type VirtualNodeSpec struct {
	MeshName         string            `json:"meshName"`
	Listeners        []Listener        `json:"listeners,omitempty"`
	ServiceDiscovery *ServiceDiscovery `json:"serviceDiscovery,omitempty"`
	Backends         []Backend         `json:"backends,omitempty"`
}
type Listener struct {
	PortMapping PortMapping `json:"portMapping"`
}
type ServiceDiscovery struct {
	Dns *DnsServiceDiscovery `json:"dns,omitempty"`
}
type DnsServiceDiscovery struct {
	HostName string `json:"hostName"`
}
type Backend struct {
	VirtualService VirtualServiceBackend `json:"virtualService"`
}
type VirtualServiceBackend struct {
	VirtualServiceName string `json:"virtualServiceName"`
}
