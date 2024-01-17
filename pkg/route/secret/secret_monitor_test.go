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
	scenarios := []struct {
		name            string
		routeSecretName string
		expectErr       bool
	}{
		{
			name:            "invalid routeSecretName: r_",
			routeSecretName: "r_",
			expectErr:       true,
		},
		{
			name:            "invalid routeSecretName: _s",
			routeSecretName: "_s",
			expectErr:       true,
		},
		{
			name:            "invalid routeSecretName: rs",
			routeSecretName: "rs",
			expectErr:       true,
		},
		{
			name:            "valid routeSecretName",
			routeSecretName: "r_s",
			expectErr:       false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			fakeKubeClient := fake.NewSimpleClientset()
			fakeInformer := func() cache.SharedInformer {
				return fakeSecretInformer(context.Background(), fakeKubeClient, "ns", "name")
			}
			key := NewObjectKey("ns", s.routeSecretName)
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}

			_, gotErr := sm.addSecretEventHandler("ns", s.routeSecretName, cache.ResourceEventHandlerFuncs{}, fakeInformer)
			if gotErr != nil && !s.expectErr {
				t.Errorf("unexpected error %v", gotErr)
			}
			if gotErr == nil && s.expectErr {
				t.Errorf("expecting an error, got nil")
			}
			if !s.expectErr {
				if _, exist := sm.monitors[key]; !exist {
					t.Error("monitor should be added into map", key)
				}
			}
		})
	}
}

func TestRemoveSecretEventHandler(t *testing.T) {
	scenarios := []struct {
		name         string
		isNilHandler bool
		expectErr    bool
	}{
		{
			name:         "nil secret handler is provided",
			isNilHandler: true,
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
				return fakeSecretInformer(context.Background(), fakeKubeClient, "ns", "name")
			}
			key := NewObjectKey("ns", "r_s")
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}
			h, err := sm.addSecretEventHandler(key.Namespace, key.Name, cache.ResourceEventHandlerFuncs{}, fakeInformer)
			if err != nil {
				t.Error(err)
			}
			if s.isNilHandler {
				h = nil
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
		testRouteName  = "testRouteName"
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
			key := NewObjectKey(testNamespace, testRouteName+"_"+testSecretName)
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}
			h, err := sm.addSecretEventHandler(key.Namespace, key.Name, cache.ResourceEventHandlerFuncs{}, fakeInformer)
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
