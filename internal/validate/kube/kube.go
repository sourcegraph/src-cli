package kube

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/sourcegraph/src-cli/internal/validate"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var (
	sourcegraphFrontend    = regexp.MustCompile(`^sourcegraph-frontend-.*`)
	sourcegraphRepoUpdater = regexp.MustCompile(`^repo-updater-.*`)
	sourcegraphWorker      = regexp.MustCompile(`^worker-.*`)
)

type Option = func(config *Config)

type Config struct {
	namespace  string
	output     io.Writer
	exitStatus bool
	clientSet  *kubernetes.Clientset
	restConfig *rest.Config
	eks        bool
	eksClient  *eks.Client
	ec2Client  *ec2.Client
}

func WithNamespace(namespace string) Option {
	return func(config *Config) {
		config.namespace = namespace
	}
}

func Quiet() Option {
	return func(config *Config) {
		config.output = io.Discard
		config.exitStatus = true
	}
}

func Eks() Option {
	eksConfig, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Printf("error while loading config: %s", err)
	}

	ec2Config, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Printf("error while loading config: %s", err)
	}

	return func(config *Config) {
		config.eks = true
		config.eksClient = eks.NewFromConfig(eksConfig)
		config.ec2Client = ec2.NewFromConfig(ec2Config)
	}
}

type validation func(ctx context.Context, config *Config) ([]validate.Result, error)

type validationGroup struct {
	Validate   validation
	WaitMsg    string
	SuccessMsg string
	ErrMsg     string
}

// Validate will call a series of validation functions in a table driven tests style.
func Validate(ctx context.Context, clientSet *kubernetes.Clientset, restConfig *rest.Config, opts ...Option) error {
	cfg := &Config{
		namespace:  "default",
		output:     os.Stdout,
		exitStatus: false,
		clientSet:  clientSet,
		restConfig: restConfig,
		eks:        false,
	}

	// this is where options are read.
	for _, opt := range opts {
		opt(cfg)
	}

	log.SetOutput(cfg.output)

	var validations []validationGroup
	validations = append(validations, validationGroup{Pods, "validating pods", "pods validated", "validating pods failed"})
	validations = append(validations, validationGroup{Services, "validating services", "services validated", "validating services failed"})
	validations = append(validations, validationGroup{PVCs, "validating pvcs", "pvcs validated", "validating pvcs failed"})
	validations = append(validations, validationGroup{Connections, "validating connections", "connections validated", "validating connections failed"})

	if cfg.eks == true {
		validations = append(validations, validationGroup{EksEbs, "EKS: validating ebs-csi drivers", "EKS: ebs-csi drivers validated", "EKS: validating ebs-csi drivers failed"})
		validations = append(validations, validationGroup{EksVpc, "EKS: validating vpc", "EKS: vpc validated", "EKS: validating vpc failed"})
	}

	var totalFailCount int

	for _, v := range validations {
		log.Printf("%s %s...", validate.HourglassEmoji, v.WaitMsg)
		results, err := v.Validate(ctx, cfg)
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

		if failCount > 0 || warnCount > 0 {
			log.Printf("\n%s %s", validate.FlashingLightEmoji, v.ErrMsg)
		}

		if failCount > 0 {
			log.Printf("  %s %d total failure(s)", validate.EmojiFingerPointRight, failCount)

			totalFailCount = totalFailCount + failCount
		}

		if warnCount > 0 {
			log.Printf("  %s %d total warning(s)", validate.EmojiFingerPointRight, warnCount)
		}

		if failCount == 0 && warnCount == 0 {
			log.Printf("%s %s!", validate.SuccessEmoji, v.SuccessMsg)
		}
	}

	if totalFailCount > 0 {
		return errors.Newf("validation failed: %d failures", totalFailCount)
	}

	return nil
}

// Pods will validate all pods in a given namespace.
func Pods(ctx context.Context, config *Config) ([]validate.Result, error) {
	pods, err := config.clientSet.CoreV1().Pods(config.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var results []validate.Result

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
				Message: fmt.Sprintf("container.Name is empty, pod '%s'", pod.Name),
			})
		}

		if container.Image == "" {
			results = append(results, validate.Result{
				Status:  validate.Failure,
				Message: fmt.Sprintf("container.Image is empty, pod '%s'", pod.Name),
			})
		}
	}

	for _, c := range pod.Status.ContainerStatuses {
		if !c.Ready {
			results = append(results, validate.Result{
				Status:  validate.Failure,
				Message: fmt.Sprintf("container '%s' is not ready, pod '%s'", c.ContainerID, pod.Name),
			})
		}

		if c.RestartCount > 50 {
			results = append(results, validate.Result{
				Status:  validate.Warning,
				Message: fmt.Sprintf("container '%s' has high restart count: %d restarts", c.ContainerID, c.RestartCount),
			})
		}
	}

	return results
}

// Services will validate all  services in a given namespace.
func Services(ctx context.Context, config *Config) ([]validate.Result, error) {
	services, err := config.clientSet.CoreV1().Services(config.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var results []validate.Result

	for _, service := range services.Items {
		r := validateService(&service)
		results = append(results, r...)
	}

	return results, nil
}

func validateService(service *corev1.Service) []validate.Result {
	var results []validate.Result

	if service.Name == "" {
		results = append(results, validate.Result{Status: validate.Failure, Message: "service.Name is empty"})
	}

	if service.Namespace == "" {
		results = append(results, validate.Result{Status: validate.Failure, Message: "service.Namespace is empty"})
	}

	if len(service.Spec.Ports) == 0 {
		results = append(results, validate.Result{Status: validate.Failure, Message: "service.Ports is empty"})
	}

	return results
}

// PVCs will validate all persistent volume claims on a given namespace
func PVCs(ctx context.Context, config *Config) ([]validate.Result, error) {
	pvcs, err := config.clientSet.CoreV1().PersistentVolumeClaims(config.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var results []validate.Result

	for _, pvc := range pvcs.Items {
		r := validatePVC(&pvc)
		results = append(results, r...)
	}

	return results, nil
}

func validatePVC(pvc *corev1.PersistentVolumeClaim) []validate.Result {
	var results []validate.Result

	if pvc.Name == "" {
		results = append(results, validate.Result{Status: validate.Failure, Message: "pvc.Name is empty"})
	}

	if pvc.Status.Phase != "Bound" {
		results = append(results, validate.Result{Status: validate.Failure, Message: "pvc.Status is not bound"})
	}

	return results
}

type connection struct {
	src  corev1.Pod
	dest []dest
}

type dest struct {
	addr string
	port string
}

// Connections will validate that Sourcegraph services can reach each other over the network.
func Connections(ctx context.Context, config *Config) ([]validate.Result, error) {
	var results []validate.Result
	var connections []connection

	pods, err := config.clientSet.CoreV1().Pods(config.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// iterate through pods looking for specific pod name prefixes, then construct
	// a relationship map between pods that should have connectivity with each other
	for _, pod := range pods.Items {
		switch name := pod.Name; {
		case sourcegraphFrontend.MatchString(name): // pod is one of the sourcegraph front-end pods
			connections = append(connections, connection{
				src: pod,
				dest: []dest{
					{
						addr: "pgsql",
						port: "5432",
					},
					{
						addr: "indexed-search",
						port: "6070",
					},
					{
						addr: "repo-updater",
						port: "3182",
					},
					{
						addr: "syntect-server",
						port: "9238",
					},
				},
			})
		case sourcegraphWorker.MatchString(name): // pod is a worker pod
			connections = append(connections, connection{
				src: pod,
				dest: []dest{
					{
						addr: "pgsql",
						port: "5432",
					},
				},
			})
		case sourcegraphRepoUpdater.MatchString(name):
			connections = append(connections, connection{
				src: pod,
				dest: []dest{
					{
						addr: "pgsql",
						port: "5432",
					},
				},
			})
		}
	}

	// use network relationships constructed above to test network connection for each relationship
	for _, c := range connections {
		for _, d := range c.dest {
			req := config.clientSet.CoreV1().RESTClient().Post().
				Resource("pods").
				Name(c.src.Name).
				Namespace(c.src.Namespace).
				SubResource("exec")

			req.VersionedParams(&corev1.PodExecOptions{
				Command: []string{"/usr/bin/nc", "-z", d.addr, d.port},
				Stdin:   false,
				Stdout:  true,
				Stderr:  true,
				TTY:     false,
			}, scheme.ParameterCodec)

			exec, err := remotecommand.NewSPDYExecutor(config.restConfig, "POST", req.URL())
			if err != nil {
				return nil, err
			}

			var stdout, stderr bytes.Buffer

			err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
				Stdout: &stdout,
				Stderr: &stderr,
			})
			if err != nil {
				return nil, err
			}

			if stderr.String() != "" {
				results = append(results, validate.Result{Status: validate.Failure, Message: stderr.String()})
			}
		}
	}

	return results, nil
}

func contains(sl *[]string, t string) bool {
	for _, s := range *sl {
		if s == t {
			return true
		}
	}
	return false
}

// EksEbs will validate that EKS cluster has ebs-cli drivers installed
func EksEbs(ctx context.Context, config *Config) ([]validate.Result, error) {
	var results []validate.Result

	if config.eksClient == nil {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: validate ebs-csi driver failed",
		})
		return results, nil
	}

	clusterName := "sourcegraph-cluster"
	inputs := &eks.ListAddonsInput{ClusterName: &clusterName}
	outputs, err := config.eksClient.ListAddons(ctx, inputs)

	if err != nil {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: validate ebs-csi driver failed",
		})
		return results, err
	}

	r := validateEbsCsi(&outputs.Addons)
	results = append(results, r...)

	return results, nil
}

func validateEbsCsi(addons *[]string) (result []validate.Result) {
	if contains(addons, "aws-ebs-csi-driver") {
		result = append(result, validate.Result{
			Status:  validate.Success,
			Message: "EKS: ebs-csi driver validated",
		})
		return result
	}

	result = append(result, validate.Result{
		Status:  validate.Failure,
		Message: "EKS: validate ebs-csi driver failed",
	})
	return result
}

// EksVpc checks if a valid vpc available
func EksVpc(ctx context.Context, config *Config) ([]validate.Result, error) {
	var results []validate.Result
	if config.ec2Client == nil {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: validate VPC failed",
		})
	}

	inputs := &ec2.DescribeVpcsInput{}
	outputs, err := config.ec2Client.DescribeVpcs(ctx, inputs)

	if err != nil {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: Validate VPC failed",
		})
		return results, nil
	}

	if len(outputs.Vpcs) == 0 {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: Validate VPC failed: No VPC configured",
		})
		return results, nil
	}

	for _, vpc := range outputs.Vpcs {
		r := validateVpc(&vpc)
		results = append(results, r...)
	}

	return results, nil
}

func validateVpc(vpc *types.Vpc) (result []validate.Result) {
	state := vpc.State

	if state == "available" {
		result = append(result, validate.Result{
			Status:  validate.Success,
			Message: "VPC is validated",
		})
	} else {
		result = append(result, validate.Result{
			Status:  validate.Failure,
			Message: "vpc.State stuck in pending state",
		})
	}
	return result
}
