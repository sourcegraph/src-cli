package kube

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/sourcegraph/src-cli/internal/validate"
    
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type EbsTestObjects struct {
	addons         []string
	serviceAccount string
	ebsRolePolicy  RolePolicy
}

// EksEbsCsiDrivers will validate that EKS cluster has ebs-cli drivers installed and configured
func EksEbsCsiDrivers(ctx context.Context, config *Config) ([]validate.Result, error) {
	var results []validate.Result
	var ebsTestParams EbsTestObjects

	addons, err := getAddons(ctx, config.eksClient)
	if err != nil {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: could not validate ebs in addons",
		})
		return results, err
	}

	ebsSA, err := getEbsSA(ctx, config.clientSet)
	if err != nil {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: could not validate ebs service account",
		})
		return results, err
	}

	EBSCSIRole, err := getEBSCSIRole(ctx, config.iamClient, ebsSA)
	if err != nil {
		results = append(results, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: could not validate ebs role policy",
		})
	}

	ebsTestParams.addons = addons
	ebsTestParams.serviceAccount = ebsSA
	ebsTestParams.ebsRolePolicy = EBSCSIRole

	r := validateEbsCsiDrivers(ebsTestParams)
	results = append(results, r...)

	return results, nil
}

func validateEbsCsiDrivers(testers EbsTestObjects) (result []validate.Result) {
	for _, addon := range testers.addons {
		if addon == "aws-ebs-csi-driver" {
			result = append(result, validate.Result{
				Status:  validate.Success,
				Message: "EKS: ebs-csi driver validated",
			})
		} else {
			result = append(result, validate.Result{
				Status:  validate.Failure,
				Message: "EKS: no 'aws-ebs-csi-driver' present in addons",
			})
		}
	}

	if testers.serviceAccount != "" {
		result = append(result, validate.Result{
			Status:  validate.Success,
			Message: "EKS: ebs service account validated",
		})
	} else {
		result = append(result, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: no 'ebs-csi-controller-sa' service account present in cluster",
		})
	}

	if *testers.ebsRolePolicy.PolicyName == "AmazonEBSCSIDriverPolicy" {
		result = append(result, validate.Result{
			Status:  validate.Success,
			Message: "EKS: service account ebs role policy validated",
		})
	} else {
		result = append(result, validate.Result{
			Status:  validate.Failure,
			Message: "EKS: no 'AmazonEBSCSIDriverPolicy' attached to role",
		})
	}

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
		return result
	}

	result = append(result, validate.Result{
		Status:  validate.Failure,
		Message: "vpc.State stuck in pending state",
	})

	return result
}

// checks if current context is set to EKS cluster
func CurrentContextSetToEKSCluster() error {
	home := homedir.HomeDir()
	pathToKubeConfig := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: pathToKubeConfig},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		}).RawConfig()

	if err != nil {
        return err
	}

	got := strings.Split(config.CurrentContext, ":")
	want := []string{"arn", "aws", "eks"}

	if len(got) >= 3 {
		got = got[:3]
		if reflect.DeepEqual(got, want) {
			return nil
		}
	}

	return errors.Newf("%s no eks cluster configured", validate.FailureEmoji)
}

func getAddons(ctx context.Context, client *eks.Client) ([]string, error) {
	clusterName := getClusterName(ctx, client)
	inputs := &eks.ListAddonsInput{ClusterName: clusterName}
	outputs, err := client.ListAddons(ctx, inputs)

	if err != nil {
		return nil, err
	}

	return outputs.Addons, nil
}

func getEbsSA(ctx context.Context, client *kubernetes.Clientset) (string, error) {
	serviceAccounts := client.CoreV1().ServiceAccounts("kube-system")
	ebsSA, err := serviceAccounts.Get(
		ctx,
		"ebs-csi-controller-sa",
		metav1.GetOptions{},
	)

	if err != nil {
		return "", err
	}

	annotations := ebsSA.GetAnnotations()
	roleArn := strings.Split(annotations["eks.amazonaws.com/role-arn"], "/")
	ebsControllerSA := roleArn[len(roleArn)-1]

	return ebsControllerSA, nil
}

type RolePolicy struct {
	PolicyName *string
	PolicyArn  *string
}

func getEBSCSIRole(ctx context.Context, client *iam.Client, SAName string) (RolePolicy, error) {
	inputs := iam.ListAttachedRolePoliciesInput{RoleName: &SAName}
	outputs, err := client.ListAttachedRolePolicies(ctx, &inputs)

	if err != nil {
		return RolePolicy{}, err
	}

	var policyName *string
	for _, policy := range outputs.AttachedPolicies {
		policyName = policy.PolicyName
		if *policyName == "AmazonEBSCSIDriverPolicy" {
			return RolePolicy{
                PolicyName: policy.PolicyName,
                PolicyArn: policy.PolicyArn,
            }, nil
		}
	}

	return RolePolicy{}, nil
}

func getClusterName(ctx context.Context, client *eks.Client) *string {
	home := homedir.HomeDir()
	pathToKubeConfig := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: pathToKubeConfig},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		}).RawConfig()

	if err != nil {
		fmt.Printf("error while checking current context: %s\n", err)
		return nil
	}

	currentContext := strings.Split(config.CurrentContext, "/")
	clusterName := currentContext[len(currentContext)-1]
	return &clusterName
}
