package discovery

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/stefanprodan/appmesh-gateway/pkg/envoy"
)

// Controller watches Kubernetes for App Mesh virtual services and
// creates or deletes Envoy clusters and virtual hosts
type Controller struct {
	client    dynamic.Interface
	indexer   cache.Indexer
	queue     workqueue.RateLimitingInterface
	informer  cache.Controller
	snapshot  *envoy.Snapshot
	vsManager *VirtualServiceManager
	vnManager *VirtualNodeManager
}

// NewController reconciles the App Mesh virtual services with Envoy clusters and virtual hosts
func NewController(client dynamic.Interface, namespace string, snapshot *envoy.Snapshot, vsManager *VirtualServiceManager, vnManager *VirtualNodeManager) *Controller {
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

	return &Controller{
		client:    client,
		informer:  informer,
		indexer:   indexer,
		queue:     queue,
		snapshot:  snapshot,
		vsManager: vsManager,
		vnManager: vnManager,
	}
}

// Run starts the App Mesh discovery controller
func (ctrl *Controller) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer ctrl.queue.ShutDown()

	go ctrl.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, ctrl.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	ctrl.syncAll()

	for i := 0; i < threadiness; i++ {
		go wait.Until(ctrl.runWorker, time.Second, stopCh)
	}

	tickChan := time.NewTicker(5 * time.Minute).C
	for {
		select {
		case <-tickChan:
			ctrl.syncAll()
		case <-stopCh:
			klog.Info("stopping Kubernetes discovery workers")
			return
		}
	}
}

func (ctrl *Controller) sync(key string) error {
	_, exists, err := ctrl.indexer.GetByKey(key)
	if err != nil {
		klog.Errorf("fetching object with key %s from store failed %v", key, err)
		return err
	}

	if !exists {
		klog.Infof("deleting %s from cache", key)
		ctrl.snapshot.Delete(key)
	}

	ctrl.syncAll()
	return nil
}

func (ctrl *Controller) syncAll() {
	var backends []string
	for _, value := range ctrl.indexer.List() {
		un := value.(*unstructured.Unstructured)
		vs, err := ctrl.vsManager.VirtualServiceFromUnstructured(un)
		if err != nil {
			klog.Errorf("unmarshal object %s from store failed %v", un.GetName(), err)
			return
		}
		if ctrl.vsManager.IsValid(*vs) {
			backends = append(backends, vs.Name)
			ctrl.snapshot.Store(fmt.Sprintf("%s/%s", vs.Namespace, vs.Name), ctrl.vsManager.ConvertToUpstream(*vs))
		}
	}

	err := ctrl.vnManager.Reconcile(backends)
	if err != nil {
		klog.Error(err)
		return
	}

	err = ctrl.snapshot.Sync()
	if err != nil {
		klog.Errorf("snapshot error %v", err)
		return
	}
}

func (ctrl *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		ctrl.queue.Forget(key)
		return
	}

	if ctrl.queue.NumRequeues(key) < 5 {
		klog.Infof("error syncing %v: %v", key, err)
		ctrl.queue.AddRateLimited(key)
		return
	}

	ctrl.queue.Forget(key)
	runtime.HandleError(err)
	klog.Infof("dropping %q out of the queue: %v", key, err)
}

func (ctrl *Controller) processNextItem() bool {
	key, quit := ctrl.queue.Get()
	if quit {
		return false
	}
	defer ctrl.queue.Done(key)

	err := ctrl.sync(key.(string))
	ctrl.handleErr(err, key)
	return true
}

func (ctrl *Controller) runWorker() {
	for ctrl.processNextItem() {
	}
}
