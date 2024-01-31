package secret

import (
	"fmt"
	"sync"
	"sync/atomic"

	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// ObjectKey represents the unique identifier for a resource.
// It is used during reading from the cache to uniquely identify and retrieve resources.
type ObjectKey struct {
	// Namespace is the namespace in which the resource is located.
	Namespace string
	// Name denotes metadata.name of a resource being monitorned by informer
	Name string
}

type singleItemMonitor struct {
	key         ObjectKey
	informer    cache.SharedInformer
	numHandlers atomic.Int32
	lock        sync.Mutex
	stopped     bool
	stopCh      chan struct{}
}

func NewObjectKey(namespace, name string) ObjectKey {
	return ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
}

func newSingleItemMonitor(key ObjectKey, informer cache.SharedInformer) *singleItemMonitor {
	return &singleItemMonitor{
		key:      key,
		informer: informer,
		stopCh:   make(chan struct{}),
	}
}

func (i *singleItemMonitor) HasSynced() bool {
	return i.informer.HasSynced()
}

func (i *singleItemMonitor) StartInformer() {
	klog.Info("starting informer")
	i.informer.Run(i.stopCh)
}

func (i *singleItemMonitor) StopInformer() bool {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.stopped {
		return false
	}
	i.stopped = true
	close(i.stopCh)
	klog.Info("informer stopped")
	return true
}

func (i *singleItemMonitor) AddEventHandler(handler cache.ResourceEventHandler) (SecretEventHandlerRegistration, error) {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.stopped {
		return nil, fmt.Errorf("can not add hanler %v to already stopped informer", handler)
	}

	registration, err := i.informer.AddEventHandler(handler)
	if err != nil {
		return nil, err
	}
	i.numHandlers.Add(1)

	return &secretEventHandlerRegistration{
		ResourceEventHandlerRegistration: registration,
		objectKey:                        i.key,
	}, nil
}

func (i *singleItemMonitor) RemoveEventHandler(handle SecretEventHandlerRegistration) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.stopped {
		return fmt.Errorf("can not remove handler %v from stopped informer", handle.GetHandler())
	}

	if err := i.informer.RemoveEventHandler(handle.GetHandler()); err != nil {
		return err
	}
	i.numHandlers.Add(-1)
	return nil
}

// GetItem returns the accumulator being monitored
// by informer, using keyFunc namespace/name
func (i *singleItemMonitor) GetItem() (item interface{}, exists bool, err error) {
	keyFunc := i.key.Namespace + "/" + i.key.Name
	return i.informer.GetStore().GetByKey(keyFunc)
}
