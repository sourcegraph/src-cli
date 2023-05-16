package usage

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
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
	metricsClient *metricsv.Clientset
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

func WithSpy(spy bool) Option {
	return func(config *Config) {
		config.spy = true
	}
}

func K8s(cxt context.Context, clientSet *kubernetes.Clientset, client *rest.Config, opts ...Option) error {
	cfg := &Config{
		namespace:     "default",
		docker:        false,
		pod:           "",
		container:     "",
		spy:           false,
		k8sClient:     clientSet,
		dockerClient:  nil,
		metricsClient: &metricsv.Clientset{},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	fmt.Println("K8s works!")
	fmt.Printf("config: %v", &cfg)
	return nil
}

func Docker(ctx context.Context, client client.Client, opts ...Option) error {
	cfg := &Config{
		namespace:     "default",
		docker:        true,
		pod:           "",
		container:     "",
		spy:           false,
		k8sClient:     nil,
		dockerClient:  &client,
		metricsClient: nil,
	}
    
	for _, opt := range opts {
		opt(cfg)
	}

	fmt.Println("docker works!")
	fmt.Printf("config: %v", &cfg)
	return nil
}

func getPercentage(x, y float64) (float64, error) {
	if x == 0 {
		return 0, nil
	}
    
	if y == 0 {
		return -1, errors.New("cannot divide by 0")
	}

	return x * 100 / y, nil
}
