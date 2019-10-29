package discovery

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"time"
)

type Controller struct {
	clientset   *kubernetes.Clientset
	indexer     cache.Indexer
	queue       workqueue.RateLimitingInterface
	informer    cache.Controller
	envoyConfig *EnvoyConfig
}

func NewController(clientset *kubernetes.Clientset, namespace string, envoyConfig *EnvoyConfig) *Controller {
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

	return &Controller{
		clientset:   clientset,
		informer:    informer,
		indexer:     indexer,
		queue:       queue,
		envoyConfig: envoyConfig,
	}
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("starting discovery workers")

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	klog.Info("stopping discovery workers")
}

func (c *Controller) sync(key string) error {
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		klog.Errorf("fetching object with key %s from store failed %v", key, err)
		return err
	}

	if !exists {
		klog.Infof("%s removed", key)
		c.envoyConfig.Delete(key)
	} else {
		klog.Infof("%s upsert", key)
		svc := obj.(*corev1.Service)
		c.envoyConfig.Upsert(key, *svc)
	}
	c.envoyConfig.Sync()
	return nil
}

func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	if c.queue.NumRequeues(key) < 5 {
		klog.Infof("error syncing %v: %v", key, err)
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	runtime.HandleError(err)
	klog.Infof("dropping %q out of the queue: %v", key, err)
}

func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync(key.(string))
	c.handleErr(err, key)
	return true
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}
