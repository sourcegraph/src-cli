package kube

import (
	"context"
	"fmt"
	"log"

	"github.com/sourcegraph/src-cli/internal/validate"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type Option = func(config *Config)

type Config struct {
	namespace string
}

func WithNamespace(namespace string) Option {
	return func(config *Config) {
		config.namespace = namespace
	}
}

type validation func(ctx context.Context, clientSet *kubernetes.Clientset, conf *Config) ([]validate.Result, error)

func Validate(ctx context.Context, clientSet *kubernetes.Clientset, opts ...Option) error {
	conf := &Config{
		namespace: "default",
	}

	for _, opt := range opts {
		opt(conf)
	}

	var validations = []struct {
		Validate   validation
		WaitMsg    string
		SuccessMsg string
		ErrMsg     string
	}{
		{Pods, "validating pods", "pods validated", "validating pods failed"},
		{Services, "validating services", "services validated", "validating services failed"},
		{Connections, "validating connections", "connections validated", "validating connections failed"},
	}

	for _, v := range validations {
		log.Printf("%s %s...", validate.HourglassEmoji, v.WaitMsg)
		results, err := v.Validate(ctx, clientSet, conf)
		if err != nil {
			return errors.Wrapf(err, v.ErrMsg)
		}

		var failCount int
		var warnCount int
		var succCount int

		for _, r := range results {
			switch r.Status {
			case validate.Failure:
				log.Printf("  %s failure: %s", validate.FailureEmoji, r.Message)
				failCount++
			case validate.Warning:
				log.Printf("  %s warning: %s", validate.WarningSign, r.Message)
				warnCount++
			case validate.Success:
				succCount++
			}
		}

		if failCount > 0 {
			log.Printf("\n%s %s", validate.FlashingLightEmoji, v.ErrMsg)
			log.Printf("  %s %d total warning(s)", validate.EmojiFingerPointRight, warnCount)
			log.Printf("  %s %d total failure(s)", validate.EmojiFingerPointRight, failCount)
		} else {
			log.Printf("%s %s!", validate.SuccessEmoji, v.SuccessMsg)
		}
	}

	return nil
}

// Pods will validate all pods in a given namespace.
func Pods(ctx context.Context, clientSet *kubernetes.Clientset, conf *Config) ([]validate.Result, error) {
	pods, err := clientSet.CoreV1().Pods(conf.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var results []validate.Result

	//TODO make concurrent
	for _, pod := range pods.Items {
		r := validatePod(&pod)
		results = append(results, r...)
	}

	return results, nil
}

func validatePod(pod *corev1.Pod) []validate.Result {
	var results []validate.Result

	if pod.Name == "" {
		results = append(results, validate.Result{Status: validate.Failure, Message: "pod.Name is empty"})
	}

	if pod.Namespace == "" {
		results = append(results, validate.Result{Status: validate.Failure, Message: "pod.Namespace is empty"})
	}

	if len(pod.Spec.Containers) == 0 {
		results = append(results, validate.Result{Status: validate.Failure, Message: "spec.Containers is empty"})
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: fmt.Sprintf("pod '%s' has a status 'pending'", pod.Name),
		})
	case corev1.PodFailed:
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: fmt.Sprintf("pod '%s' has a status 'failed'", pod.Name),
		})
	}

	for _, container := range pod.Spec.Containers {
		if container.Name == "" {
			results = append(results, validate.Result{
				Status:  validate.Failure,
				Message: fmt.Sprintf("container.Name is emtpy, pod '%s'", pod.Name),
			})
		}

		if container.Image == "" {
			results = append(results, validate.Result{
				Status:  validate.Failure,
				Message: fmt.Sprintf("container.Image is emtpy, pod '%s'", pod.Name),
			})
		}
	}

	for _, c := range pod.Status.ContainerStatuses {
		if !c.Ready {
			results = append(results, validate.Result{
				Status:  validate.Failure,
				Message: fmt.Sprintf("container '%s' is not ready, pod '%s'", c.Name, pod.Name),
			})
		}

		if c.RestartCount > 50 {
			results = append(results, validate.Result{
				Status:  validate.Warning,
				Message: fmt.Sprintf("container '%s' has high restart count: %d restarts", c.Name, c.RestartCount),
			})
		}
	}

	return results
}

// Services will validate all  services in a given namespace.
func Services(ctx context.Context, clientSet *kubernetes.Clientset, conf *Config) ([]validate.Result, error) {
	services, err := clientSet.CoreV1().Services(conf.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var results []validate.Result

	//TODO make concurrent
	for _, service := range services.Items {
		r := validateService(&service)
		results = append(results, r...)
	}

	return results, nil
}

func validateService(service *corev1.Service) []validate.Result {
	return nil
}

// Connections will validate that Sourcegraph services can reach each other.
func Connections(ctx context.Context, clientSet *kubernetes.Clientset, conf *Config) ([]validate.Result, error) {
	return nil, nil
}

func validateConnection() []validate.Result {
	return nil
}
