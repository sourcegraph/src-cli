package usage

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
    
)


type Option = func(config *Config)
type Config struct {
	namespace     string
	pod           string
	container     string
	spy           bool
	docker        bool
	k8sClient     *kubernetes.Clientset
	dockerClient  *client.Client
	metricsClient *metricsv
}

func WithNamespace(namespace string) Option {
    return func(config *Config) {
        config.namespace = namespace
    }
}

func WithPod(podname string) Option {
    return func(config *Config) {
        config.pod = podname
    }
}

func WithContainer(containerName string) Option {
    return func(config *Config) {
        config.container = containerName
    }
}

func K8s(context context.Context, clientSet kubernetes.Clientset, restConfig *rest.Config, opts ...Option) error {
    cfg := &Config{
        namespace: "default",
        docker: false,
        pod: "",
        container: "",
        k8sClient: &clientSet,
        dockerClient: nil,
    }

    
    
    fmt.Println("K8s works!")
    return nil
}

func getPercentage(x, y float64) (float64, error) {
    if x == 0 { return 0, nil }
    if y == 0 { return -1, errors.New("cannot divide by 0") }
    
    return x * 100 / y, nil
}


