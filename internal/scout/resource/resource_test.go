package resource

import (
	"context"
	"fmt"

	/* "fmt"
	"io"
	"os"
	"strings" */
	"testing"
	// "text/tabwriter"

	// "github.com/docker/docker/api/types"
	// "github.com/docker/docker/api/types/container"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	// "github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func TestResourcesK8s(t *testing.T) {
	ctx := context.Background()

	config, err := clientcmd.BuildConfigFromFlags("", homedir.HomeDir()+"/.kube/config")
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

	err = K8s(ctx, k8sClientSet, nil, WithNamespace("test"))
	if err != nil {
		t.Fatal(errors.Wrap(err, "Error calling ResourcesK8s"))
	}
}

/* func TestGetResourceInfo(t *testing.T) {
	cases := []struct {
		name      string
		container func(*types.ContainerJSONBase)
		expected  string
	}{
		{
			name: "bad format: bad input",
			container: func(container *types.ContainerJSONBase) {
				container.Name = "container1"
				container.HostConfig.Resources.NanoCPUs = 2000000000
				container.HostConfig.Resources.CPUShares = 512
				container.HostConfig.Resources.Memory = 1536870912
				container.HostConfig.Resources.MemoryReservation = 268435456
			},
			expected: "",
		},
	}

	for _, tc := range cases {
		// why does this have to be here to work?
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			container := containerHelper()
			if tc.container != nil {
				tc.container(container)
			}
			// TODO: logic for testing
			// call function then validate the output
		})
	}
}

func containerHelper() *types.ContainerJSONBase {
	return &types.ContainerJSONBase{
		Name: "",
		HostConfig: &container.HostConfig{
			Resources: container.Resources{
				NanoCPUs:          0,
				CPUShares:         0,
				Memory:            0,
				MemoryReservation: 0,
			},
		},
	}
} */

func TestGetMemUnits(t *testing.T) {
	cases := []struct {
		name      string
		param     int64
		wantUnit  string
		wantValue int64
		wantError error
	}{
		{
			name:      "convert bytes below a million to KB",
			param:     999999,
			wantUnit:  "KB",
			wantValue: 999999,
			wantError: nil,
		},
		{
			name:      "convert bytes below a billion to MB",
			param:     999999999,
			wantUnit:  "MB",
			wantValue: 999,
			wantError: nil,
		},
		{
			name:      "convert bytes above a billion to GB",
			param:     12999999900,
			wantUnit:  "GB",
			wantValue: 12,
			wantError: nil,
		},
		{
			name:      "return error for a negative number",
			param:     -300,
			wantUnit:  "",
			wantValue: -300,
			wantError: fmt.Errorf("invalid memory value: %d", -300),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotUnit, gotValue, gotError := getMemUnits(tc.param)

			if gotUnit != tc.wantUnit {
				t.Errorf("got %s want %s", gotUnit, tc.wantUnit)
			}

			if gotValue != tc.wantValue {
				t.Errorf("got %v want %v", gotValue, tc.wantValue)
			}

            if gotError == nil && tc.wantError != nil {
                t.Error("got nil want error")
            }

            if gotError != nil && tc.wantError == nil {
                t.Error("got error want nil")
            }

			return
		})
	}

}
