package secret

import (
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type SecretEventHandlerRegistration interface {
	cache.ResourceEventHandlerRegistration

	GetKey() ObjectKey
	GetHandler() cache.ResourceEventHandlerRegistration
}

type SecretMonitor interface {
	//
	AddSecretEventHandler(namespace, secretName string, handler cache.ResourceEventHandler) (SecretEventHandlerRegistration, error)
	//
	RemoveSecretEventHandler(SecretEventHandlerRegistration) error
	//
	GetSecret(SecretEventHandlerRegistration) (*v1.Secret, error)
}

type secretEventHandlerRegistration struct {
	cache.ResourceEventHandlerRegistration
	// objectKey will populate during AddEventHandler, and will be used during RemoveEventHandler
	objectKey ObjectKey
}

func (r *secretEventHandlerRegistration) GetKey() ObjectKey {
	return r.objectKey
}

func (r *secretEventHandlerRegistration) GetHandler() cache.ResourceEventHandlerRegistration {
	return r.ResourceEventHandlerRegistration
}

type secretMonitor struct {
	kubeClient kubernetes.Interface

	lock sync.RWMutex
	// monitors is map of singleItemMonitor. Each singleItemMonitor monitors/watches
	// a secret through individual informer.
	monitors map[ObjectKey]*singleItemMonitor
}

func NewSecretMonitor(kubeClient kubernetes.Interface) SecretMonitor {
	return &secretMonitor{
		kubeClient: kubeClient,
		monitors:   map[ObjectKey]*singleItemMonitor{},
	}
}

// create secret watch.
func (s *secretMonitor) AddSecretEventHandler(namespace, secretName string, handler cache.ResourceEventHandler) (SecretEventHandlerRegistration, error) {
	return s.addSecretEventHandler(namespace, secretName, handler, nil)
}

// addSecretEventHandler should only be used directly for tests. For production use AddSecretEventHandler().
// createInformerFn helps in mocking sharedInformer for unit tests.
func (s *secretMonitor) addSecretEventHandler(namespace, secretName string, handler cache.ResourceEventHandler, createInformerFn func() cache.SharedInformer) (SecretEventHandlerRegistration, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// secret identifier (namespace/secret)
	key := NewObjectKey(namespace, secretName)

	// check if secret monitor(watch) already exists
	m, exists := s.monitors[key]

	// start secret informer
	if !exists {
		var sharedInformer cache.SharedInformer
		if createInformerFn == nil {
			// create a single secret monitor
			sharedInformer = cache.NewSharedInformer(
				cache.NewListWatchFromClient(
					s.kubeClient.CoreV1().RESTClient(),
					"secrets",
					namespace,
					fields.OneTermEqualSelector("metadata.name", secretName),
				),
				&corev1.Secret{},
				0,
			)
		} else {
			// only for testability
			klog.Warning("creating informer for testability")
			sharedInformer = createInformerFn()
		}

		m = newSingleItemMonitor(key, sharedInformer)
		go m.StartInformer()

		// add item key to monitors map // add watch to the list
		s.monitors[key] = m

		klog.Info("secret informer started", " item key ", key)
	}

	// secret informer already started, just add the handler
	klog.Info("secret handler added", " item key ", key)

	return m.AddEventHandler(handler) // also populate key inside secretEventHandlerRegistration
}

// Remove secret watch
func (s *secretMonitor) RemoveSecretEventHandler(handlerRegistration SecretEventHandlerRegistration) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if handlerRegistration == nil {
		return fmt.Errorf("secret handler is nil")
	}

	// get the secret identifier linked with handler
	// populated in AddEventHandler()
	key := handlerRegistration.GetKey()

	// check if secret informer already exists for the secret(key)
	m, exists := s.monitors[key]
	if !exists {
		klog.Info("secret monitor already removed", " item key", key)
		return nil
		// TODO return error
	}

	if err := m.RemoveEventHandler(handlerRegistration); err != nil {
		return err
	}
	klog.Info("secret handler removed", " item key", key)

	// stop informer if there is no handler
	if m.numHandlers.Load() <= 0 {
		if !m.StopInformer() {
			klog.Error("secret informer already stopped", " item key", key)
		}
		delete(s.monitors, key)
		klog.Info("secret informer stopped", " item key ", key)
	}

	return nil
}

// Get the secret object from informer's cache
func (s *secretMonitor) GetSecret(handlerRegistration SecretEventHandlerRegistration) (*v1.Secret, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if handlerRegistration == nil {
		return nil, fmt.Errorf("secret handler is nil")
	}
	key := handlerRegistration.GetKey()
	secretName := key.Name

	// check if secret informer exists
	m, exists := s.monitors[key]
	if !exists {
		return nil, fmt.Errorf("secret monitor doesn't exist for key %v", key)
	}

	// TODO: secretName should not be required
	uncast, exists, err := m.GetItem(secretName)
	if !exists {
		return nil, fmt.Errorf("secret %s doesn't exist in cache", secretName)
	}

	if err != nil {
		return nil, err
	}

	secret, ok := uncast.(*v1.Secret)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", uncast)
	}

	return secret, nil
}
