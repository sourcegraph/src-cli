package resources

import (
	"context"
	"fmt"
	"math"
	"os"
	"text/tabwriter"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Option = func(config *Config)

type Config struct {
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

// ResourcesK8s prints the CPU and memory resource limits and requests for all pods in the given namespace.
func K8s(ctx context.Context, clientSet *kubernetes.Clientset, restConfig *rest.Config, opts ...Option) error {
	cfg := &Config{
		namespace:    "default",
		docker:       false,
		k8sClient:    clientSet,
		dockerClient: nil,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return listPodResources(ctx, cfg)
}

func listPodResources(ctx context.Context, cfg *Config) error {
	podInterface := cfg.k8sClient.CoreV1().Pods(cfg.namespace)
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

				capacity, err := getPVCCapacity(ctx, cfg, container, pod)
				if err != nil {
					return err
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

func getPVCCapacity(ctx context.Context, cfg *Config, container v1.Container, pod v1.Pod) (string, error) {
	var capacity string
	for _, volumeMount := range container.VolumeMounts {
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == volumeMount.Name && volume.PersistentVolumeClaim != nil {
				pvc, err := cfg.k8sClient.CoreV1().PersistentVolumeClaims(cfg.namespace).Get(
					ctx,
					volume.PersistentVolumeClaim.ClaimName,
					metav1.GetOptions{},
				)

				if err != nil {
					return "", errors.Wrapf(
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
	return capacity, nil
}

// DockerClientInterface defines the interface for interacting with the Docker API.
type DockerClientInterface interface {
	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	Close() error
}

// ResourcesDocker prints the CPU and memory resource limits and requests for running Docker containers.
func Docker(ctx context.Context, dockerClient DockerClientInterface) error {
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return fmt.Errorf("error listing docker containers: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Container\tCPU Limits\tCPU Period\tCPU Quota\tMem Limits\tMem Requests")

	for _, container := range containers {
		containerInfo, err := dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			return fmt.Errorf("Error inspecting container %s: %v", container.ID, err)
		}

		cpuLimits := containerInfo.HostConfig.NanoCPUs
		cpuPeriod := containerInfo.HostConfig.CPUPeriod
		cpuQuota := containerInfo.HostConfig.CPUQuota
		memLimits := containerInfo.HostConfig.Memory
		memRequests := containerInfo.HostConfig.MemoryReservation

		limUnit, limVal, err := getMemUnits(memLimits)
		if err != nil {
			return err
		}

		reqUnit, reqVal, err := getMemUnits(memRequests)
		if err != nil {
			return err
		}

		fmt.Fprintf(
			w,
			"%s\t%d\t%v\t%v\t%v\t%v\n",
			containerInfo.Name,
			cpuLimits/1e9,
			fmt.Sprintf("%d MS", cpuPeriod/1000),
			fmt.Sprintf(`%v%%`, math.Ceil((float64(cpuQuota)/float64(cpuPeriod))*100)),
			fmt.Sprintf("%d %s", limVal, limUnit),
			fmt.Sprintf("%d %s", reqVal, reqUnit),
		)
	}

	w.Flush()
	return nil
}

// getMemUnits converts a byte value to the appropriate memory unit.
func getMemUnits(valToConvert int64) (string, int64, error) {
	if valToConvert < 0 {
		return "", valToConvert, fmt.Errorf("invalid memory value: %d", valToConvert)
	}

	var memUnit string
	if valToConvert < 1000000 {
		memUnit = "KB"
	} else if valToConvert < 1000000000 {
		memUnit = "MB"
		valToConvert = valToConvert / 1000000
	} else {
		memUnit = "GB"
		valToConvert = valToConvert / 1000000000
	}

	return memUnit, valToConvert, nil
}
