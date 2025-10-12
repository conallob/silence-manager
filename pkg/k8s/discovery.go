package k8s

import (
	"context"
	"fmt"
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// DiscoveryConfig holds configuration for Alertmanager service discovery
type DiscoveryConfig struct {
	ServiceName      string // Service name pattern to match (e.g., "alertmanager")
	ServiceLabel     string // Label selector (e.g., "app=alertmanager")
	Port             int    // Port to connect to (default: 9093)
	PreferNamespaces []string // Preferred namespaces to search first
}

// DiscoveredService represents a discovered Alertmanager service
type DiscoveredService struct {
	Name      string
	Namespace string
	URL       string
}

// DiscoverAlertmanager discovers Alertmanager services across all namespaces
func DiscoverAlertmanager(cfg DiscoveryConfig) (*DiscoveredService, error) {
	// Create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ctx := context.Background()

	// Search for services
	var discoveredServices []DiscoveredService

	// First, try preferred namespaces if specified
	if len(cfg.PreferNamespaces) > 0 {
		for _, ns := range cfg.PreferNamespaces {
			services, err := findServicesInNamespace(ctx, clientset, ns, cfg)
			if err != nil {
				log.Printf("Warning: failed to search namespace %s: %v", ns, err)
				continue
			}
			discoveredServices = append(discoveredServices, services...)
			if len(discoveredServices) > 0 {
				log.Printf("Found Alertmanager in preferred namespace: %s", ns)
				break
			}
		}
	}

	// If not found in preferred namespaces, search all namespaces
	if len(discoveredServices) == 0 {
		log.Println("Searching all namespaces for Alertmanager services...")

		// List all namespaces
		namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list namespaces: %w", err)
		}

		for _, ns := range namespaces.Items {
			// Skip already-searched preferred namespaces
			if contains(cfg.PreferNamespaces, ns.Name) {
				continue
			}

			services, err := findServicesInNamespace(ctx, clientset, ns.Name, cfg)
			if err != nil {
				log.Printf("Warning: failed to search namespace %s: %v", ns.Name, err)
				continue
			}
			discoveredServices = append(discoveredServices, services...)
		}
	}

	// Return results
	if len(discoveredServices) == 0 {
		return nil, fmt.Errorf("no Alertmanager services found in cluster")
	}

	// Log all discovered services
	log.Printf("Discovered %d Alertmanager service(s):", len(discoveredServices))
	for i, svc := range discoveredServices {
		log.Printf("  %d. %s/%s - %s", i+1, svc.Namespace, svc.Name, svc.URL)
	}

	// Return the first discovered service
	selected := discoveredServices[0]
	log.Printf("Selected Alertmanager: %s/%s - %s", selected.Namespace, selected.Name, selected.URL)

	return &selected, nil
}

// findServicesInNamespace searches for Alertmanager services in a specific namespace
func findServicesInNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string, cfg DiscoveryConfig) ([]DiscoveredService, error) {
	var discovered []DiscoveredService

	// Try label selector first if provided
	if cfg.ServiceLabel != "" {
		services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: cfg.ServiceLabel,
		})
		if err == nil && len(services.Items) > 0 {
			for _, svc := range services.Items {
				if ds := serviceToDiscovered(svc, cfg.Port); ds != nil {
					discovered = append(discovered, *ds)
				}
			}
		}
	}

	// If no services found by label, try by name pattern
	if len(discovered) == 0 && cfg.ServiceName != "" {
		services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}

		for _, svc := range services.Items {
			// Match service name (case-insensitive contains)
			if strings.Contains(strings.ToLower(svc.Name), strings.ToLower(cfg.ServiceName)) {
				if ds := serviceToDiscovered(svc, cfg.Port); ds != nil {
					discovered = append(discovered, *ds)
				}
			}
		}
	}

	return discovered, nil
}

// serviceToDiscovered converts a Kubernetes service to a DiscoveredService
func serviceToDiscovered(svc corev1.Service, preferredPort int) *DiscoveredService {
	// Determine the port to use
	port := preferredPort
	if port == 0 {
		port = 9093 // Default Alertmanager port
	}

	// Verify the service has the port we're looking for
	portFound := false
	for _, p := range svc.Spec.Ports {
		if int(p.Port) == port || p.Name == "web" || p.Name == "http" {
			port = int(p.Port)
			portFound = true
			break
		}
	}

	// If preferred port not found, use first available port
	if !portFound && len(svc.Spec.Ports) > 0 {
		port = int(svc.Spec.Ports[0].Port)
	}

	// Build URL
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, port)

	return &DiscoveredService{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		URL:       url,
	}
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
