package secret

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

var (
	testNamespace  = "testNamespace"
	testSecretName = "testSecretName"
)

func fakeSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"test": {},
		},
	}
}

/*
- Scenarios
	- Informer is running properly
	- Informer is stopped properly
	 	- Dual stop should return false
	- AddEventHandler should increase the count by 1, check error
	- RemoveEventHandler should decrease the count by 1, check error
*/

func TestMonitor(t *testing.T) {
	fakeKubeClient := fake.NewSimpleClientset(fakeSecret(testSecretName))
	queue := make(chan string)

	sharedInformer := cache.NewSharedInformer(&cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return fakeKubeClient.CoreV1().Secrets(testNamespace).List(context.TODO(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return fakeKubeClient.CoreV1().Secrets(testNamespace).Watch(context.TODO(), options)
		},
	}, &corev1.Secret{}, 1*time.Second)

	singleItemMonitor := newSingleItemMonitor(ObjectKey{}, sharedInformer)

	_, err := singleItemMonitor.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				t.Errorf("invalid object")
			}
			queue <- secret.Name
		},
	})

	if err != nil {
		t.Errorf("got error %v", err)
	}
	go singleItemMonitor.StartInformer()

	// wait for informer to sync
	time.Sleep(time.Second)
	if !singleItemMonitor.HasSynced() {
		t.Fatal("infromer not synced yet")
	}

	select {
	case s := <-queue:
		if s != testSecretName {
			t.Errorf("expected %s got %s", testSecretName, s)
		}
		singleItemMonitor.Stop()
	case <-time.After(3 * time.Second):
		t.Fatal("test timeout")
	}
}
