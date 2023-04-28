package resources

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	_ "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Option = func(config *Config)

type Config struct {
	kubeConfig   *string
	namespace    string
	docker       bool
	k8sClient    *kubernetes.Clientset
	dockerClient *client.Client
}

func WithNamespace(namespace string) Option {
	return func(config *Config) {
		config.namespace = namespace
	}
}

func UsesDocker() Option {
	return func(config *Config) {
		config.docker = true
	}
}

func ResourcesK8s(ctx context.Context, clientSet *kubernetes.Clientset, restConfig *rest.Config, opts ...Option) error {
	cfg := &Config{
		namespace:    "default",
		docker:       false,
		k8sClient:    clientSet,
		dockerClient: nil,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	podInterface := clientSet.CoreV1().Pods(cfg.namespace)
	podList, err := podInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "Error listing pods: ")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "POD\tCPU LIMITS\tCPU REQUESTS\tMEM LIMITS\tMEM REQUESTS\tDISK")

	for _, pod := range podList.Items {
		if pod.GetNamespace() == cfg.namespace {
			for _, container := range pod.Spec.Containers {
				cpuLimits := container.Resources.Limits.Cpu()
				cpuRequests := container.Resources.Requests.Cpu()
				memLimits := container.Resources.Limits.Memory()
				memRequests := container.Resources.Requests.Memory()
				fmt.Fprintf(
					w,
					"%s\t%s\t%s\t%s\t%s\t\n",
					container.Name,
					cpuLimits,
					cpuRequests,
					memLimits,
					memRequests,
				)
			}
		}
	}

	w.Flush()

	return nil
}
