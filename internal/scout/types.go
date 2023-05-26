package scout

import (
	"github.com/docker/docker/client"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Config struct {
	Namespace     string
	Pod           string
	Container     string
	Spy           bool
	Docker        bool
	RestConfig    *rest.Config
	K8sClient     *kubernetes.Clientset
	DockerClient  *client.Client
	MetricsClient *metricsv.Clientset
}
