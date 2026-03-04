package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

var (
	once          sync.Once
	clientset     *kubernetes.Clientset
	metricsClient *metricsv.Clientset
	initErr       error
)

func initClients() {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home, _ := os.UserHomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			initErr = fmt.Errorf("failed to build k8s config: %w", err)
			return
		}
	}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		initErr = fmt.Errorf("failed to create k8s client: %w", err)
		return
	}

	// Best-effort metrics client
	metricsClient, _ = metricsv.NewForConfig(config)
}

// Client returns the shared Kubernetes clientset.
func Client() (*kubernetes.Clientset, error) {
	once.Do(initClients)
	return clientset, initErr
}

// MetricsClient returns the shared metrics clientset (may be nil).
func MetricsClient() *metricsv.Clientset {
	once.Do(initClients)
	return metricsClient
}
