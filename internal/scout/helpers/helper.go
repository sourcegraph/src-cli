package helper

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetPods(ctx context.Context, k8sClient *kubernetes.Clientset, namespace string) ([]corev1.Pod, error) {
	podInterface := k8sClient.CoreV1().Pods(namespace)
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
