package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/sourcegraph/src-cli/internal/validate/kube"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func init() {
	usage := `'src validate kube' is a tool that validates a Kubernetes based Sourcegraph deployment
`

	flagSet := flag.NewFlagSet("kube", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src validate %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		namespace = flagSet.String("namespace", "", "specify the kubernetes namespace to use")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		var kubeConfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeConfig = flagSet.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeConfig = flagSet.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}

		if err := flagSet.Parse(args); err != nil {
			return err
		}

		// use the current context in kubeConfig
		config, err := clientcmd.BuildConfigFromFlags("", *kubeConfig)
		if err != nil {
			return errors.Wrap(err, "failed to kubernetes config")
		}

		clientSet, err := kubernetes.NewForConfig(config)
		if err != nil {
			return errors.Wrap(err, "failed to create kubernetes client")
		}

		// if a user specifies a namespace
		if namespace != nil {
			ns := *namespace
			return kube.Validate(context.Background(), clientSet, kube.WithNamespace(ns))
		}

		// no namespace specified
		return kube.Validate(context.Background(), clientSet)

	}

	validateCommands = append(validateCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
