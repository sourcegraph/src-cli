package kube

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func Contains(sl *[]string, t string) bool {
	for _, s := range *sl {
		if s == t {
			return true
		}
	}
	return false
}

// checks if current context is set to EKS cluster
func CurrentContextSetToEKSCluster() bool {
	home := homedir.HomeDir()
	pathToKubeConfig := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: pathToKubeConfig},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		}).RawConfig()

	if err != nil {
		fmt.Printf("error while checking current context: %s\n", err)
		return false
	}

	got := strings.Split(config.CurrentContext, ":")
	want := []string{"arn", "aws", "eks"}

	if len(got) >= 3 {
		got := got[:3]
		if reflect.DeepEqual(got, want) {
			return true
		}
	}

	return false
}
