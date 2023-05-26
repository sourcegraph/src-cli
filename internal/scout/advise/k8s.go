package advise

import (
	"context"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	helper "github.com/sourcegraph/src-cli/internal/scout/helpers"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

func K8s(
    ctx context.Context, 
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
    
    pods, err := helper.GetPods(ctx, cfg.k8sClient, cfg.namespace)
    if err != nil {
        return errors.Wrap(err, "could not get list of pods")
    }

    PrintPods(pods)
    return nil
}


func PrintPods(pods []v1.Pod) {
    for _, p := range pods {
        fmt.Println(p.Name)
    }
}
