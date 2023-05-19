package usage

import (
	"github.com/docker/docker/client"
	"k8s.io/client-go/kubernetes"
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
