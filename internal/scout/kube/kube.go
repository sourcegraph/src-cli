package kube

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout"
	"gopkg.in/inf.v0"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	metav1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// GetPods returns a list of Pod objects in a given Kubernetes namespace.
//
// It accepts:
// - ctx: The context for the request
// - k8sClient: A Kubernetes clientset for making API requests
// - namespace: The Kubernetes namespace to list Pods from
//
// It returns:
// - A slice of Pod objects from the given namespace
// - An error if there was an issue listing Pods
//
// If no Pods exist in the given namespace, a warning message is printed and the
// program exits with a non-zero status code.
func GetPods(ctx context.Context, cfg *scout.Config) ([]corev1.Pod, error) {
	podInterface := cfg.K8sClient.CoreV1().Pods(cfg.Namespace)
	podList, err := podInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return []corev1.Pod{}, errors.Wrap(err, "could not list pods")
	}

	if len(podList.Items) == 0 {
		msg := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
		fmt.Println(msg.Render(`
            No pods exist in this namespace.
            Did you mean to use the --namespace flag?

            If you are attempting to check
            resources for a docker deployment, you
            must use the --docker flag.
            See --help for more info.
            `))
		os.Exit(1)
	}

	return podList.Items, nil
}

// getPod returns a Pod object with the given name.
//
// If a Pod with the given name exists in pods, it is returned.
// Otherwise, an error is returned indicating no Pod with that name exists.
func GetPod(podName string, pods []corev1.Pod) (corev1.Pod, error) {
	for _, p := range pods {
		if p.Name == podName {
			return p, nil
		}
	}
	return corev1.Pod{}, errors.New("no pod with this name exists in this namespace")
}

// getPodMetrics retrieves metrics for a given pod.
//
// It returns:
// - podMetrics: The PodMetrics object containing metrics for the pod
// - err: Any error that occurred while getting the pod metrics
func GetPodMetrics(ctx context.Context, cfg *scout.Config, pod corev1.Pod) (*metav1beta1.PodMetrics, error) {
	podMetrics, err := cfg.MetricsClient.
		MetricsV1beta1().
		PodMetricses(cfg.Namespace).
		Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pod metrics")
	}

	return podMetrics, nil
}

// GetRawUsage retrieves the raw usage value for a given resource type from a Kubernetes ResourceList.
//
// It accepts:
// - usages: A Kubernetes ResourceList containing usage values for multiple resource types
// - targetKey: The key for the resource type to retrieve usage for (e.g. "cpu" or "memory")
//
// It returns:
// - The raw usage value for the target resource type as a float64
// - An error if there was an issue retrieving or parsing the usage value
func GetRawUsage(usages corev1.ResourceList, targetKey string) (float64, error) {
	var usage *inf.Dec

	for key, val := range usages {
		if key.String() == targetKey {
			usage = val.AsDec().SetScale(0)
		}
	}

	toFloat, err := strconv.ParseFloat(usage.String(), 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert inf.Dec type to float")
	}

	return toFloat, nil
}

// getPvcCapacity retrieves the storage capacity of a PersistentVolumeClaim
// mounted as a volume by a container.
//
// It returns:
// - The capacity Quantity of the PVC if a matching PVC mount is found
// - nil if no PVC mount is found
// - Any error that occurred while getting the PVC
func GetPvcCapacity(ctx context.Context, cfg *scout.Config, container corev1.Container, pod *corev1.Pod) (*resource.Quantity, error) {
	for _, vm := range container.VolumeMounts {
		for _, v := range pod.Spec.Volumes {
			if v.Name == vm.Name && v.PersistentVolumeClaim != nil {
				pvc, err := cfg.K8sClient.
					CoreV1().
					PersistentVolumeClaims(cfg.Namespace).
					Get(
						ctx,
						v.PersistentVolumeClaim.ClaimName,
						metav1.GetOptions{},
					)
				if err != nil {
					return nil, errors.Wrapf(
						err,
						"failed to get PVC %s",
						v.PersistentVolumeClaim.ClaimName,
					)
				}
				return pvc.Status.Capacity.Storage().ToDec(), nil
			}
		}
	}
	return nil, nil
}

// getStorageUsage executes the df -h command in a container and parses the
// output to get the storage usage percentage for ephemeral storage volumes.
//
// It returns:
// - The storage usage percentage for storage volumes
// - "-" if no storage volumes are found
// - Any error that occurred while executing the df -h command or parsing the output
func GetStorageUsage(
	ctx context.Context,
	cfg *scout.Config,
	podName, containerName string,
) (float64, float64, error) {
	req := cfg.K8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(cfg.Namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   []string{"df"},
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg.RestConfig, "POST", req.URL())
	if err != nil {
		return 0, 0, err
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return 0, 0, err
	}

	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines[1 : len(lines)-1] {
		fields := strings.Fields(line)

		if acceptedFileSystem(fields[0]) {
			capacityFloat, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return 0, 0, errors.Wrap(err, "could not convert string to float64")
			}

			usageFloat, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				return 0, 0, errors.Wrap(err, "could not convert string to float64")
			}
			return capacityFloat, usageFloat, nil
		}
	}

	return 0, 0, nil
}

// acceptedFileSystem checks if a given file system, represented
// as a string, is an accepted system.
//
// It returns:
// - True if the file system matches the pattern '/dev/sd[a-z]$'
// - False otherwise
func acceptedFileSystem(fileSystem string) bool {
	matched, _ := regexp.MatchString(`/dev/sd[a-z]$`, fileSystem)
	return matched
}
