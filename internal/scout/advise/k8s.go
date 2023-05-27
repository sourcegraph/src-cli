package advise

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/scout"
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
	cfg := &scout.Config{
		Namespace:     "default",
		Pod:           "",
		Container:     "",
		Spy:           false,
		Docker:        false,
		RestConfig:    restConfig,
		K8sClient:     k8sClient,
		DockerClient:  nil,
		MetricsClient: metricsClient,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	/* pods, err := kube.GetPods(ctx, cfg)
	if err != nil {
		return errors.Wrap(err, "could not get list of pods")
	} */

	/* if cfg.Pod != "" {
		pod, err := kube.GetPod(cfg.Pod, pods)
		if err != nil {
			return errors.Wrap(err, "could not get pod")
		}

	} */
	fmt.Println("advise.K8s needs code")
	return nil
}

/* func Advise(ctx context.Context, cfg *scout.Config, pod v1.Pod) error {
    fmt.Println(pod.Name)
    return nil
} */
