package usage

import (
	"github.com/sourcegraph/src-cli/internal/scout"
)

type Option = func(config *scout.Config)

func WithNamespace(namespace string) Option {
	return func(config *scout.Config) {
		config.Namespace = namespace
	}
}

func WithPod(podname string) Option {
	return func(config *scout.Config) {
		config.Pod = podname
	}
}

func WithContainer(containerName string) Option {
	return func(config *scout.Config) {
		config.Container = containerName
	}
}

func WithSpy(spy bool) Option {
	return func(config *scout.Config) {
		config.Spy = true
	}
}
// contains checks if a string slice contains a given value.
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// getPercentage calculates the percentage of x in relation to y.
func getPercentage(x, y float64) float64 {
	if x == 0 {
		return 0
	}

	if y == 0 {
		return 0
	}

	return x * 100 / y
}
