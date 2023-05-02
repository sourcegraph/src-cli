package resources

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/stretchr/testify/mock"
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

type DockerClientMock struct {
	mock.Mock
}

func (m *DockerClientMock) ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	args := m.Called(ctx, options)
	return args.Get(0).([]types.Container), args.Error(1)
}

func (m *DockerClientMock) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	args := m.Called(ctx, containerID)
	return args.Get(0).(types.ContainerJSON), args.Error(1)
}

func (m *DockerClientMock) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestResourcesDocker(t *testing.T) {
	dockerClient := new(DockerClientMock)

	dockerClient.On("ContainerList", mock.Anything, mock.Anything).Return([]types.Container{
		{ID: "container1"},
		{ID: "container2"},
		{ID: "container3"},
	}, nil)

	dockerClient.On("ContainerInspect", mock.Anything, "container1").Return(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Name: "container1",
			HostConfig: &container.HostConfig{
				Resources: container.Resources{
					NanoCPUs:          2000000000,
					CPUPeriod:         100000,
					CPUQuota:          50000,
					Memory:            1536870912,
					MemoryReservation: 268435456,
				},
			},
		},
	}, nil)

	dockerClient.On("ContainerInspect", mock.Anything, "container2").Return(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Name: "container2",
			HostConfig: &container.HostConfig{
				Resources: container.Resources{
					NanoCPUs:          1000000000,
					CPUPeriod:         50000,
					CPUQuota:          25000,
					Memory:            268435456,
					MemoryReservation: 134217728,
				},
			},
		},
	}, nil)

	dockerClient.On("ContainerInspect", mock.Anything, "container3").Return(types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Name: "container3",
			HostConfig: &container.HostConfig{
				Resources: container.Resources{
					NanoCPUs:          4000000000,
					CPUPeriod:         150000,
					CPUQuota:          65000,
					Memory:            5268435456,
					MemoryReservation: 4134217728,
				},
			},
		},
	}, nil)

	dockerClient.On("Close").Return(nil)

	var expectedOutput strings.Builder
	expectedW := tabwriter.NewWriter(&expectedOutput, 0, 0, 2, ' ', 0)

	fmt.Fprintln(expectedW, "Container\tCPU Limits\tCPU Period\tCPU Quota\tMem Limits\tMem Requests")
	fmt.Fprintf(expectedW, "container1\t2\t100 MS\t50%%\t1 GB\t268 MB\n")
	fmt.Fprintf(expectedW, "container2\t1\t50 MS\t50%%\t268 MB\t134 MB\n")
	fmt.Fprintf(expectedW, "container3\t4\t150 MS\t44%%\t5 GB\t4 GB\n")
	expectedW.Flush()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Docker(context.Background(), dockerClient)
	if err != nil {
		t.Fatalf("ResourcesDocker returned an error: %v", err)
	}

	err = dockerClient.Close()
	if err != nil {
		t.Fatalf("Error closing docker client: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("Error reading from pipe: %v", err)
	}

	if string(output) != expectedOutput.String() {
		t.Errorf("Expected output:\n%s\nActual output:\n%s", expectedOutput.String(), output)
	}

	dockerClient.AssertExpectations(t)
}
