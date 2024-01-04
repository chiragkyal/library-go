package secret

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

// import (
// 	"context"
// 	"testing"

// 	corev1 "k8s.io/api/core/v1"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/fields"
// 	"k8s.io/client-go/kubernetes/fake"
// 	"k8s.io/client-go/tools/cache"
// )

// var (
// 	testNamespace  = "testNamespace"
// 	testSecretName = "testSecretName"
// )

func fakeMonitor(kubeClient *fake.Clientset, stopCh chan struct{}) *singleItemMonitor {
	sharedInformer := cache.NewSharedInformer(
		cache.NewListWatchFromClient(
			kubeClient.CoreV1().RESTClient(),
			"secrets",
			testNamespace,
			fields.OneTermEqualSelector("metadata.name", testSecretName),
		),
		&corev1.Secret{},
		0,
	)

	return newSingleItemMonitor(ObjectKey{}, sharedInformer)
}

// func fakeSecret() *corev1.Secret {
// 	return &corev1.Secret{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      testSecretName,
// 			Namespace: testNamespace,
// 		},
// 		Data: map[string][]byte{
// 			"test": {},
// 		},
// 	}
// }

// func TestStart(t *testing.T) {
// 	kubeClient := fake.NewSimpleClientset()
// 	ch := make(chan struct{})
// 	m := fakeMonitor(kubeClient, ch)

// 	m.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
// 		AddFunc: func(obj interface{}) {
// 			t.Logf("add is called")
// 		},
// 	})

// 	go func() {
// 		m.Start()
// 	}()

// 	t.Log(m.informer.HasSynced())

// 	if _, err := kubeClient.CoreV1().Secrets(testNamespace).Create(context.Background(), fakeSecret(), metav1.CreateOptions{}); err != nil {
// 		t.Fatalf("failed to create fake secret: %v", err)
// 	}

// 	t.Error(m.informer.GetStore().List())

// 	// select {
// 	// case <-m.stopCh:
// 	// 	m.Stop()
// 	// case <-time.After(3 * time.Second):
// 	// 	t.Fatal("test timeout")
// 	// }
// }

// func TestStop(t *testing.T) {
// 	kubeClient := fake.NewSimpleClientset()
// 	ch := make(chan struct{})
// 	m := fakeMonitor(kubeClient, ch)

// 	m.Start()

// 	// time.Sleep(2 * time.Second)

// 	// test if is running and not stopped

// 	if !m.Stop() {
// 		t.Errorf("failed to stop informer")
// 	}

// }
