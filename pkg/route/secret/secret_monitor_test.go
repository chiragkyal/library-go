package secret

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestAddSecretEventHandler(t *testing.T) {
	var (
		namespace  = "ns"
		secretName = "secret"
	)

	scenarios := []struct {
		name              string
		handler           cache.ResourceEventHandler
		numInvocation     int
		expectNumHandlers int32
		expectKey         ObjectKey
		expectErr         int
	}{
		{
			name:              "nil handler is provided",
			handler:           nil,
			numInvocation:     1,
			expectNumHandlers: 0,
			expectErr:         1,
		},
		{
			name:              "correct handler is provided",
			handler:           cache.ResourceEventHandlerFuncs{},
			numInvocation:     2,
			expectKey:         NewObjectKey(namespace, secretName),
			expectNumHandlers: 2,
			expectErr:         0,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			fakeKubeClient := fake.NewSimpleClientset()
			fakeInformer := func() cache.SharedInformer {
				return fakeSecretInformer(context.TODO(), fakeKubeClient, namespace, secretName)
			}
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}

			gotErr := 0
			for i := 0; i < s.numInvocation; i++ {
				if _, err := sm.addSecretEventHandler(context.TODO(), namespace, secretName, s.handler, fakeInformer); err != nil {
					gotErr += 1
				}
			}
			if gotErr != s.expectErr {
				t.Errorf("expected %d errors, got %d errors", s.expectErr, gotErr)
			}

			if s.expectErr == 0 {
				if _, exist := sm.monitors[s.expectKey]; !exist {
					t.Fatal("monitor key should be added into map", s.expectKey)
				}
				if sm.monitors[s.expectKey].numHandlers.Load() != s.expectNumHandlers {
					t.Errorf("expected %d handlers, got %d handlers", s.expectNumHandlers, sm.monitors[s.expectKey].numHandlers.Load())
				}
			}
		})
	}
}

func TestRemoveSecretEventHandler(t *testing.T) {
	scenarios := []struct {
		name         string
		isNilHandler bool
		isKeyRemoved bool
		expectErr    bool
	}{
		{
			name:         "nil secret handler is provided",
			isNilHandler: true,
			expectErr:    true,
		},
		{
			name:         "secret monitor key already removed",
			isKeyRemoved: true,
			expectErr:    true,
		},
		{
			name:      "secret handler correctly removed",
			expectErr: false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			fakeKubeClient := fake.NewSimpleClientset()
			fakeInformer := func() cache.SharedInformer {
				return fakeSecretInformer(context.TODO(), fakeKubeClient, "ns", "name")
			}
			key := NewObjectKey("ns", "secret")
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}
			h, err := sm.addSecretEventHandler(context.TODO(), key.Namespace, key.Name, cache.ResourceEventHandlerFuncs{}, fakeInformer)
			if err != nil {
				t.Error(err)
			}
			if s.isNilHandler {
				h = nil
			}
			if s.isKeyRemoved {
				delete(sm.monitors, key)
			}

			gotErr := sm.RemoveSecretEventHandler(h)
			if gotErr != nil && !s.expectErr {
				t.Errorf("unexpected error %v", gotErr)
			}
			if gotErr == nil && s.expectErr {
				t.Errorf("expecting an error, got nil")
			}
		})
	}
}

func TestGetSecret(t *testing.T) {
	var (
		testNamespace  = "testNamespace"
		testSecretName = "testSecretName"
		secret         = fakeSecret(testNamespace, testSecretName)
	)

	scenarios := []struct {
		name                  string
		isNilHandler          bool
		withSecret            bool
		expectSecretFromCache *corev1.Secret
		expectErr             bool
	}{
		{
			name:                  "secret exists in cluster but nil handler is provided",
			isNilHandler:          true,
			withSecret:            true,
			expectSecretFromCache: nil,
			expectErr:             true,
		},
		{
			// this case may occur when handler is not removed correctly
			// when secret gets deleted
			name:                  "secret does not exist in cluster and correct handler is provided",
			withSecret:            false,
			expectSecretFromCache: nil,
			expectErr:             true,
		},
		{
			name:                  "secret exists and correct handler is provided",
			withSecret:            true,
			expectSecretFromCache: secret,
			expectErr:             false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			var fakeKubeClient *fake.Clientset
			if s.withSecret {
				fakeKubeClient = fake.NewSimpleClientset(secret)
			} else {
				fakeKubeClient = fake.NewSimpleClientset()
			}

			fakeInformer := func() cache.SharedInformer {
				return fakeSecretInformer(context.TODO(), fakeKubeClient, testNamespace, testSecretName)
			}
			key := NewObjectKey(testNamespace, testSecretName)
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}
			h, err := sm.addSecretEventHandler(context.TODO(), key.Namespace, key.Name, cache.ResourceEventHandlerFuncs{}, fakeInformer)
			if err != nil {
				t.Error(err)
			}

			if !cache.WaitForCacheSync(context.Background().Done(), h.HasSynced) {
				t.Error("cache not synced yet")
			}

			if s.isNilHandler {
				h = nil
			}

			gotSec, gotErr := sm.GetSecret(h)
			if gotErr != nil && !s.expectErr {
				t.Errorf("unexpected error %v", gotErr)
			}
			if gotErr == nil && s.expectErr {
				t.Errorf("expecting an error, got nil")
			}
			if !reflect.DeepEqual(s.expectSecretFromCache, gotSec) {
				t.Errorf("expected %v got %v", s.expectSecretFromCache, gotSec)
			}
		})
	}
}
