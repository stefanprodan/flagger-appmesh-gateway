package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stefanprodan/kxds/pkg/envoy"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// KubernetesDiscovery watches Kubernetes for services events and
// pushes them to Envoy over xDS
type KubernetesDiscovery struct {
	clientset *kubernetes.Clientset
	indexer   cache.Indexer
	queue     workqueue.RateLimitingInterface
	informer  cache.Controller
	snapshot  *envoy.Snapshot
	portName  string
}

// NewKubernetesDiscovery starts watching for Kubernetes services events
func NewKubernetesDiscovery(clientset *kubernetes.Clientset, namespace string, snapshot *envoy.Snapshot, portName string) *KubernetesDiscovery {
	watch := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "services", namespace, fields.Everything())
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	indexer, informer := cache.NewIndexerInformer(watch, &corev1.Service{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			newSvc := new.(*corev1.Service)
			oldSvc := old.(*corev1.Service)
			if newSvc.ResourceVersion == oldSvc.ResourceVersion {
				return
			}
			key, err := cache.MetaNamespaceKeyFunc(new)
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
	}, cache.Indexers{})

	return &KubernetesDiscovery{
		clientset: clientset,
		informer:  informer,
		indexer:   indexer,
		queue:     queue,
		snapshot:  snapshot,
		portName:  portName,
	}
}

// Run starts the Kubernetes discovery controller
func (kd *KubernetesDiscovery) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer kd.queue.ShutDown()

	go kd.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, kd.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	kd.syncAll()

	for i := 0; i < threadiness; i++ {
		go wait.Until(kd.runWorker, time.Second, stopCh)
	}

	tickChan := time.NewTicker(5 * time.Minute).C
	for {
		select {
		case <-tickChan:
			kd.syncAll()
		case <-stopCh:
			klog.Info("stopping Kubernetes discovery workers")
			return
		}
	}
}

func (kd *KubernetesDiscovery) sync(key string) error {
	obj, exists, err := kd.indexer.GetByKey(key)
	if err != nil {
		klog.Errorf("fetching object with key %s from store failed %v", key, err)
		return err
	}

	if !exists {
		klog.Infof("deleting %s from cache", key)
		kd.snapshot.Delete(key)
	} else {
		svc := obj.(*corev1.Service)
		if kd.svcIsValid(*svc) {
			klog.Infof("storing %s in cache", key)
			kd.snapshot.Store(key, kd.svcToUpstream(*svc))
		}
	}
	kd.snapshot.Sync()
	return nil
}

func (kd *KubernetesDiscovery) syncAll() {
	for _, value := range kd.indexer.List() {
		svc := value.(*corev1.Service)
		if kd.svcIsValid(*svc) {
			//dumpUpstream(kd.svcToUpstream(*svc))
			kd.snapshot.Store(fmt.Sprintf("%s/%s", svc.Namespace, svc.Name), kd.svcToUpstream(*svc))
		}
	}
	klog.Infof("refreshing cache for %d services", kd.snapshot.Len())
	kd.snapshot.Sync()
}

func (kd *KubernetesDiscovery) handleErr(err error, key interface{}) {
	if err == nil {
		kd.queue.Forget(key)
		return
	}

	if kd.queue.NumRequeues(key) < 5 {
		klog.Infof("error syncing %v: %v", key, err)
		kd.queue.AddRateLimited(key)
		return
	}

	kd.queue.Forget(key)
	runtime.HandleError(err)
	klog.Infof("dropping %q out of the queue: %v", key, err)
}

func (kd *KubernetesDiscovery) processNextItem() bool {
	key, quit := kd.queue.Get()
	if quit {
		return false
	}
	defer kd.queue.Done(key)

	err := kd.sync(key.(string))
	kd.handleErr(err, key)
	return true
}

func (kd *KubernetesDiscovery) runWorker() {
	for kd.processNextItem() {
	}
}

// svcToUpstream converts the Kubernetes service to an Upstream
func (kd *KubernetesDiscovery) svcToUpstream(svc corev1.Service) envoy.Upstream {
	port := uint32(80)
	for _, p := range svc.Spec.Ports {
		if p.Name == kd.portName {
			port = uint32(p.Port)
		}
	}

	up := envoy.Upstream{
		Name: fmt.Sprintf("%s-%s-%d", svc.Name, svc.Namespace, port),
		Domains: []string{
			fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
			fmt.Sprintf("%s.%s:%d", svc.Name, svc.Namespace, port),
			fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace),
			fmt.Sprintf("%s.%s.svc:%d", svc.Name, svc.Namespace, port),
			fmt.Sprintf("%s.%s.svc.cluster", svc.Name, svc.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster:%d", svc.Name, svc.Namespace, port),
			fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, port),
		},
		Port:    port,
		Host:    fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),
		Prefix:  "/",
		Retries: 2,
		Timeout: 15 * time.Second,
		Canary:  &envoy.Canary{},
	}

	appendDomain := func(slice []string, i string) []string {
		for _, ele := range slice {
			if ele == i {
				return slice
			}
		}
		return append(slice, i)
	}

	for key, value := range svc.Annotations {
		if key == envoy.AnDomain {
			up.Domains = appendDomain(up.Domains, value)
		}
		if key == envoy.AnTimeout {
			d, err := time.ParseDuration(value)
			if err == nil {
				up.Timeout = d
			}
		}
		if key == envoy.AnRetries {
			r, err := strconv.Atoi(value)
			if err == nil {
				up.Retries = uint32(r)
			}
		}
		if key == envoy.AnPrimary {
			up.Canary.PrimaryCluster = value
		}
		if key == envoy.AnCanary {
			up.Canary.CanaryCluster = value
		}
		if key == envoy.AnCanaryWeight {
			r, err := strconv.Atoi(value)
			if err == nil {
				up.Canary.CanaryWeight = r
			}
		}
	}
	return up
}

// svcIsValid checks if a Kubernetes service is eligible
func (kd *KubernetesDiscovery) svcIsValid(svc corev1.Service) bool {
	var port int32
	for _, p := range svc.Spec.Ports {
		if p.Name == kd.portName {
			port = p.Port
		}
	}
	if port == 0 {
		return false
	}

	for key, value := range svc.Annotations {
		if key == envoy.AnExpose && value == "false" {
			return false
		}
	}
	return true
}

func dumpUpstream(upstream envoy.Upstream) {
	b, _ := json.Marshal(upstream)
	var out bytes.Buffer
	json.Indent(&out, b, "", "  ")
	fmt.Println(string(out.Bytes()))
}
