package resources

import (
	"context"
	"testing"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestResourcesK8s(t *testing.T) {
	ctx := context.Background()

	config, err := clientcmd.BuildConfigFromFlags("", "/Users/sourcegraph/.kube/config")
	if err != nil {
		t.Fatal(errors.Wrap(err, "Error getting in cluster config"))
	}

	k8sClientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(errors.Wrap(err, "Error creating kubernetes clientset"))
	}

	// Create some test pods to list
	pod1 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: "test",
		},
	}

	pod2 := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod2",
			Namespace: "other",
		},
	}

	k8sClientSet.CoreV1().Pods("test").Create(ctx, pod1, metav1.CreateOptions{})
	k8sClientSet.CoreV1().Pods("other").Create(ctx, pod2, metav1.CreateOptions{})

	err = ResourcesK8s(ctx, k8sClientSet, nil, WithNamespace("test"))
	if err != nil {
		t.Fatal(errors.Wrap(err, "Error calling ResourcesK8s"))
	}
}
