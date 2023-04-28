package resources

import (
	"context"
	"fmt"

	_ "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/src-cli/internal/scout"
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

	// Get a PodInterface for all namespaces (use an empty string "" as the namespace).
	podInterface := clientSet.CoreV1().Pods(cfg.namespace)

	// List all pods.
	podList, err := podInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "Error listing pods: ")
	}

	// Iterate over the list of pods and print their names and namespaces.
	for _, pod := range podList.Items {
		if pod.GetNamespace() == cfg.namespace {
			if len(pod.Spec.Containers) > 1 {
				fmt.Printf("%s %s:\n", scout.EmojiFingerPointRight, pod.Name)
				for _, container := range pod.Spec.Containers {
					cpuLimits := container.Resources.Limits.Cpu()
					cpuRequests := container.Resources.Requests.Cpu()
					memLimits := container.Resources.Limits.Memory()
					memRequests := container.Resources.Requests.Memory()
                    fmt.Printf(
                        "\t%s: \n\t\tCPU: (%v, %v), \n\t\tMemory: (%v, %v)\n",
                        container.Name, 
                        cpuLimits,
                        cpuRequests,
                        memLimits, 
                        memRequests,
                    )
				}
			} else if len(pod.Spec.Containers) == 1 {
                fmt.Printf("%s %s: ", scout.EmojiFingerPointRight, pod.Name)
                c := pod.Spec.Containers[0]
				cpuLimits := c.Resources.Limits.Cpu()
				cpuRequests := c.Resources.Requests.Cpu()
				memLimits := c.Resources.Limits.Memory()
				memRequests := c.Resources.Requests.Memory()
                fmt.Printf(
                    "\n\tCPU: (%v, %v), \n\tLimits: (%v, %v)\n", 
                    cpuLimits, 
                    cpuRequests, 
                    memLimits, 
                    memRequests,
                )
			}
		}
	}  

	return nil
}
