package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceToDiscovered(t *testing.T) {
	tests := []struct {
		name          string
		service       corev1.Service
		preferredPort int
		expectedURL   string
		expectedPort  int
	}{
		{
			name: "Service with preferred port",
			service: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alertmanager",
					Namespace: "monitoring",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 9093, Name: "web"},
					},
				},
			},
			preferredPort: 9093,
			expectedURL:   "http://alertmanager.monitoring.svc.cluster.local:9093",
			expectedPort:  9093,
		},
		{
			name: "Service with web port name",
			service: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alertmanager",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 8080, Name: "web"},
					},
				},
			},
			preferredPort: 9093,
			expectedURL:   "http://alertmanager.default.svc.cluster.local:8080",
			expectedPort:  8080,
		},
		{
			name: "Service with http port name",
			service: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alertmanager",
					Namespace: "kube-system",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 8888, Name: "http"},
					},
				},
			},
			preferredPort: 9093,
			expectedURL:   "http://alertmanager.kube-system.svc.cluster.local:8888",
			expectedPort:  8888,
		},
		{
			name: "Service with multiple ports - preferred exists",
			service: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alertmanager",
					Namespace: "monitoring",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 8080, Name: "metrics"},
						{Port: 9093, Name: "web"},
					},
				},
			},
			preferredPort: 9093,
			expectedURL:   "http://alertmanager.monitoring.svc.cluster.local:9093",
			expectedPort:  9093,
		},
		{
			name: "Service with multiple ports - preferred doesn't exist",
			service: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alertmanager",
					Namespace: "monitoring",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 8080, Name: "metrics"},
						{Port: 9090, Name: "api"},
					},
				},
			},
			preferredPort: 9093,
			expectedURL:   "http://alertmanager.monitoring.svc.cluster.local:8080",
			expectedPort:  8080,
		},
		{
			name: "Service with default port (0)",
			service: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alertmanager",
					Namespace: "monitoring",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Port: 9093},
					},
				},
			},
			preferredPort: 0,
			expectedURL:   "http://alertmanager.monitoring.svc.cluster.local:9093",
			expectedPort:  9093,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serviceToDiscovered(tt.service, tt.preferredPort)

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if result.Name != tt.service.Name {
				t.Errorf("Expected name '%s', got '%s'", tt.service.Name, result.Name)
			}

			if result.Namespace != tt.service.Namespace {
				t.Errorf("Expected namespace '%s', got '%s'", tt.service.Namespace, result.Namespace)
			}

			if result.URL != tt.expectedURL {
				t.Errorf("Expected URL '%s', got '%s'", tt.expectedURL, result.URL)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "Item exists",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "Item doesn't exist",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "Empty slice",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "Item at beginning",
			slice:    []string{"x", "y", "z"},
			item:     "x",
			expected: true,
		},
		{
			name:     "Item at end",
			slice:    []string{"x", "y", "z"},
			item:     "z",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDiscoveryConfig(t *testing.T) {
	cfg := DiscoveryConfig{
		ServiceName:      "alertmanager",
		ServiceLabel:     "app=alertmanager",
		Port:             9093,
		PreferNamespaces: []string{"monitoring", "default"},
	}

	if cfg.ServiceName != "alertmanager" {
		t.Errorf("Expected service name 'alertmanager', got '%s'", cfg.ServiceName)
	}
	if cfg.ServiceLabel != "app=alertmanager" {
		t.Errorf("Expected service label 'app=alertmanager', got '%s'", cfg.ServiceLabel)
	}
	if cfg.Port != 9093 {
		t.Errorf("Expected port 9093, got %d", cfg.Port)
	}
	if len(cfg.PreferNamespaces) != 2 {
		t.Errorf("Expected 2 preferred namespaces, got %d", len(cfg.PreferNamespaces))
	}
}

func TestDiscoveredService(t *testing.T) {
	ds := DiscoveredService{
		Name:      "alertmanager",
		Namespace: "monitoring",
		URL:       "http://alertmanager.monitoring.svc.cluster.local:9093",
	}

	if ds.Name != "alertmanager" {
		t.Errorf("Expected name 'alertmanager', got '%s'", ds.Name)
	}
	if ds.Namespace != "monitoring" {
		t.Errorf("Expected namespace 'monitoring', got '%s'", ds.Namespace)
	}
	if ds.URL != "http://alertmanager.monitoring.svc.cluster.local:9093" {
		t.Errorf("Expected URL 'http://alertmanager.monitoring.svc.cluster.local:9093', got '%s'", ds.URL)
	}
}

// Note: Testing DiscoverAlertmanager and findServicesInNamespace would require
// either a running Kubernetes cluster or more complex mocking with fake.Clientset.
// These tests are integration tests and should be run in a test environment with
// a Kubernetes cluster or using the client-go fake package.
//
// Example structure for integration tests:
//
// func TestDiscoverAlertmanager_Integration(t *testing.T) {
//     if testing.Short() {
//         t.Skip("Skipping integration test")
//     }
//     // Use fake.NewSimpleClientset() from k8s.io/client-go/kubernetes/fake
//     // to create a fake Kubernetes client for testing
// }
