package resources

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/docker/docker/api/types"
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

// ResourcesK8s prints the CPU, memory, and storage resource limits, requests and capacity for Kubernetes pods.
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
	fmt.Fprintln(w, "CONTAINER\tCPU LIMITS\tCPU REQUESTS\tMEM LIMITS\tMEM REQUESTS\tCAPACITY")

	for _, pod := range podList.Items {
		if pod.GetNamespace() == cfg.namespace {
			for _, container := range pod.Spec.Containers {
				cpuLimits := container.Resources.Limits.Cpu()
				cpuRequests := container.Resources.Requests.Cpu()
				memLimits := container.Resources.Limits.Memory()
				memRequests := container.Resources.Requests.Memory()

				var capacity string
				for _, volumeMount := range container.VolumeMounts {
					for _, volume := range pod.Spec.Volumes {
						fmt.Println(pod.Spec.Volumes)
						if volume.Name == volumeMount.Name && volume.PersistentVolumeClaim != nil {
							pvc, err := clientSet.CoreV1().PersistentVolumeClaims(cfg.namespace).Get(
								ctx,
								volume.PersistentVolumeClaim.ClaimName,
								metav1.GetOptions{},
							)

							if err != nil {
								return errors.Wrapf(
									err,
									"Error getting PVC %s",
									volume.PersistentVolumeClaim.ClaimName,
								)
							}

							capacity = pvc.Status.Capacity.Storage().String()
							break
						}
					}
				}

				fmt.Fprintf(
					w,
					"%s\t%s\t%s\t%s\t%s\t%s\t\n",
					container.Name,
					cpuLimits,
					cpuRequests,
					memLimits,
					memRequests,
					capacity,
				)
			}
		}
	}

	w.Flush()
	return nil
}

// DockerClientInterface defines the interface for interacting with the Docker API.
type DockerClientInterface interface {
	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	Close() error
}

// ResourcesDocker prints the CPU and memory resource limits and requests for running Docker containers.
func ResourcesDocker(ctx context.Context, dockerClient DockerClientInterface) error {
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return fmt.Errorf("Error listing Docker containers: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Container\tCPU Limits\tCPU Requests\tMem Limits\tMem Requests")

	for _, container := range containers {
		containerInfo, err := dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			return fmt.Errorf("Error inspecting container %s: %v", container.ID, err)
		}

		cpuLimits := containerInfo.HostConfig.NanoCPUs
		cpuRequests := containerInfo.HostConfig.CPUPeriod
		memLimits := containerInfo.HostConfig.Memory
		memRequests := containerInfo.HostConfig.MemoryReservation
		fmt.Fprintf(
			w,
			"%s\t%d\t%d\t%d\t%d\n",
			containerInfo.Name,
			cpuLimits,
			cpuRequests,
			memLimits,
			memRequests,
		)
	}

	w.Flush()
	return nil
}
