package kube

import (
	"context"
	"log"
	"strings"

	"cloud.google.com/go/container"

	"github.com/sourcegraph/src-cli/internal/validate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClusterInfo struct {
	ServiceType string
	ProjectId   string
	Region      string
	ClusterName string
}

func Gke(ctx context.Context) Option {
	gcpClient, err := container.NewClient(ctx, "beatrix-test-overlay")
	if err != nil {
		log.Println("error while loading config: ", err)
	}

	return func(config *Config) {
		config.gke = true
		config.gcpClient = gcpClient
	}
}

func GkeGcePersistentDiskCSIDrivers(ctx context.Context, config *Config) ([]validate.Result, error) {
	var result []validate.Result
	storageClient := config.clientSet.StorageV1()
    storageClasses, err := storageClient.StorageClasses().List(ctx, metav1.ListOptions{})
    
	if err != nil {
		return nil, err
	}

	for _, item := range storageClasses.Items {
		if item.Name == "sourcegraph" {
			if item.Provisioner == "kubernetes.io/gce-pd" {
				result = append(result, validate.Result{
					Status:  validate.Success,
					Message: "persistent volume provisioner present",
				})
				return result, nil
			}
		}
	}
    
	result = append(result, validate.Result{
		Status:  validate.Failure,
		Message: "persistent volume provisioner not present on sourcegraph storageclass",
	})

	return result, nil
}

func GetClusterInfo(currentContextString string) *ClusterInfo {
	clusterValues := strings.Split(currentContextString, "_")

	clusterInfo := ClusterInfo{
		ServiceType: clusterValues[0],
		ProjectId:   clusterValues[1],
		Region:      clusterValues[2],
		ClusterName: clusterValues[3],
	}

	return &clusterInfo
}
