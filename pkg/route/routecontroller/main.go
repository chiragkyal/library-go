package main

import (
	"flag"
	"fmt"
	"reflect"
	"time"

	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	routev1 "github.com/openshift/api/route/v1"
	routev1client "github.com/openshift/client-go/route/clientset/versioned"
	"github.com/openshift/library-go/pkg/route/secret"
)

var namespace = "sandbox"

// Controller demonstrates how to implement a controller with client-go.
type Controller struct {
	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller
}

// NewController creates a new Controller.
func NewController(queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller, clientset *kubernetes.Clientset /*handlerFuncs cache.ResourceEventHandlerFuncs*/) *Controller {

	return &Controller{
		informer: informer,
		indexer:  indexer,
		queue:    queue,
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	if key, ok := key.(string); ok {
		klog.Info("skipping processing route with name ", " key ", key)
		return true
	}

	// Invoke the method containing the business logic
	err := c.syncToStdout(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

// syncToStdout is the business logic of the controller. In this controller it simply prints
// information about the pod to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (c *Controller) syncToStdout(key string) error {
	obj, exists, err := c.indexer.Get(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		// Below we will warm up our cache with a Pod, so that we will see a delete for one pod
		fmt.Printf("Route %s does not exist anymore\n", key)
	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a Pod was recreated with the same name
		fmt.Printf("Sync/Add/Update for Route %s\n", obj.(*routev1.Route).GetName())
	}
	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		klog.Infof("Error syncing pod %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	utilruntime.HandleError(err)
	klog.Infof("Dropping pod %q out of the queue: %v", key, err)
}

// Run begins watching and syncing.
func (c *Controller) Run(workers int, stopCh chan struct{}) {
	defer utilruntime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	klog.Info("Starting Route controller")

	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	klog.Info("Stopping Pod controller")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func localRegisterRoute(secretManager *secret.Manager, route *routev1.Route) error {
	secreth := generateHandler(secretManager, route)
	secretManager.WithSecretHandler(secreth)
	return secretManager.RegisterRoute(route.Namespace, route.Name, route.Spec.TLS.ExternalCertificate.Name)

}
func generateHandler(secretManager *secret.Manager, route *routev1.Route) cache.ResourceEventHandlerFuncs {
	// secret handler
	secreth := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, _ := cache.MetaNamespaceKeyFunc(obj)
			secret := obj.(*v1.Secret)
			klog.Info("Secret added ", "obj ", secret.ResourceVersion, " key ", key)

			// enqueue route
			// TODO: make ResourceChanges public
			// secretManager.ResourceChanges.Add(route)
			secretManager.Queue().Add(route)
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, _ := cache.MetaNamespaceKeyFunc(new)
			secretOld := old.(*v1.Secret)
			secretNew := new.(*v1.Secret)
			klog.Info("Secret updated ", "old ", secretOld.ResourceVersion, " new ", secretNew.ResourceVersion, " key ", key)

			// enqueue route
			// TODO: make ResourceChanges public
			// secretManager.ResourceChanges.Add(route)
			secretManager.Queue().Add(route)
		},
		DeleteFunc: func(obj interface{}) {
			key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			secret := obj.(*v1.Secret)
			klog.Info("Secret deleted ", " obj ", secret.ResourceVersion, " key ", key)

			// enqueue route
			// TODO: make ResourceChanges public
			// secretManager.ResourceChanges.Add(route)
			secretManager.Queue().Add(route)
		},
	}

	return secreth
}

func main() {
	var kubeconfig string
	var master string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&master, "master", "", "master url")
	flag.Parse()

	// creates the connection
	config, err := clientcmd.BuildConfigFromFlags(master, kubeconfig)
	if err != nil {
		klog.Fatal(err)
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err)
	}

	routeListWatcher := cache.NewListWatchFromClient(routev1client.NewForConfigOrDie(config).RouteV1().RESTClient(), "routes", namespace, fields.Everything())

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	secretManager := secret.NewManager(clientset, queue)

	// Route handler
	routeh := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, _ := cache.MetaNamespaceKeyFunc(obj)

			route := obj.(*routev1.Route)
			klog.Info("Add Event ", "route.Name ", route.Name, " key ", key)

			// generate Handler and watch
			err := localRegisterRoute(secretManager, route)
			if err != nil {
				klog.Error("failed to register route")
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)

			oldRoute := old.(*routev1.Route)
			newRoute := new.(*routev1.Route)
			klog.Info("Route Update event")
			if err == nil && !reflect.DeepEqual(oldRoute.Spec, newRoute.Spec) {

				klog.Info("Roue Update event ", "old ", oldRoute.ResourceVersion, " new ", newRoute.ResourceVersion, " key ", key)
				// queue.Add(key)

				if err := secretManager.UnregisterRoute(oldRoute.Namespace, oldRoute.Name); err != nil {
					klog.Error(err)
				} // remove old watch

				if err := localRegisterRoute(secretManager, newRoute); err != nil {
					klog.Error(err)
				} // create new watch
			}

			// klog.Info("Roue Update event, No change ", "trying to GetSecret ", "new ", newRoute.Name, " referenced secret", newRoute.Spec.TLS.ExternalCertificate.Name)

			// s, err := secretManager.GetSecret(newRoute)
			// if err == nil {
			// 	klog.Info("fetching secret data ", s.Data)
			// } else {
			// 	klog.Error(err)
			// }
		},
		DeleteFunc: func(obj interface{}) {
			key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)

			route := obj.(*routev1.Route)
			klog.Info("Delete event ", " obj ", route.ResourceVersion, " key ", key)
			// if err == nil {
			// 	queue.Add(key)
			// }
			// when route is deleted, remove associated secret watcher
			err := secretManager.UnregisterRoute(route.Namespace, route.Name)
			if err != nil {
				klog.Error(err)
			}
		},
	}

	// Route Controller
	indexer, informer := cache.NewIndexerInformer(routeListWatcher, &routev1.Route{}, 0, routeh, cache.Indexers{})

	controller := NewController(queue, indexer, informer, clientset)

	// Now let's start the controller
	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(1, stop)

	// Wait forever
	select {}
}
