package advise

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

func K8s(
    context context.Context, 
    k8sClient *kubernetes.Clientset,
    metricsClient *metricsv.Clientset,
    restConfig *rest.Config,
    opts ...Option,
) error {
	cfg := &Config{
		namespace:     "default",
		docker:        false,
		pod:           "",
		container:     "",
		restConfig:    restConfig,
		k8sClient:     k8sClient,
		dockerClient:  nil,
		metricsClient: metricsClient,
	}

    for _, opt := range opts {
        opt(cfg)
    }
    
    fmt.Println("advise for k8s needs code")

    return nil
}
