package secret

import (
	"testing"

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
			key := NewObjectKey("ns", s.routeSecretName)
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}

			_, gotErr := sm.AddSecretEventHandler("ns", s.routeSecretName, cache.ResourceEventHandlerFuncs{})
			if gotErr != nil && !s.expectErr {
				t.Errorf("unexpected error %v", gotErr)
			}
			if gotErr == nil && s.expectErr {
				t.Errorf("expecting an error, got nil")
			}
			if !s.expectErr {
				if _, exist := sm.monitors[key]; !exist {
					t.Error("monitor should be added into map", key)
				} else {
					// stop informer to present memory leakage
					sm.monitors[key].StopInformer()
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
			key := NewObjectKey("ns", "r_s")
			sm := secretMonitor{
				kubeClient: fakeKubeClient,
				monitors:   map[ObjectKey]*singleItemMonitor{},
			}
			h, err := sm.AddSecretEventHandler(key.Namespace, key.Name, cache.ResourceEventHandlerFuncs{})
			if err != nil {
				t.Error(err)
			}
			if s.isNilHandler {
				// stop informer to present memory leakage
				sm.monitors[key].StopInformer()
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
