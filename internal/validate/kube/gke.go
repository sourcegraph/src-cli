package kube

import (
	"context"
	"log"
	"strings"

	"cloud.google.com/go/container"
	"google.golang.org/api/gkehub/v1"

	"github.com/sourcegraph/src-cli/internal/validate"
)

type ClusterInfo struct {
    ServiceType string
    ProjectId string
    Region string
    ClusterName string
}

func Gke(ctx context.Context) Option {
	gcpClient, err := container.NewClient(ctx, "beatrix-test-overlay")
	if err != nil {
		log.Println("error while loading config: ", err)
	}

    gkeClient, err := gkehub.NewService(ctx)
    if err != nil {
        log.Println("error while loading config: ", err)
    }

	return func(config *Config) {
		config.gke = true
		config.gcpClient = gcpClient
        config.gkeClient = gkeClient
	}
}

func GkeGcePersistentDiskCSIDrivers(ctx context.Context, config *Config)  ([]validate.Result, error) {
    /* currentContext, err := GetCurrentContext()
    if err != nil {
        return []validate.Result{}, err
    }
    
    clusterValues := GetClusterInfo(currentContext)
    cluster, err := config.gcpClient.Cluster(
        ctx, 
        clusterValues.Region, 
        clusterValues.ClusterName,
    )

    if err != nil {
        return []validate.Result{}, err
    }
 */
	return []validate.Result{}, nil
}

func GetClusterInfo(currentContextString string) *ClusterInfo {
    clusterValues := strings.Split(currentContextString, "_")
    
    clusterInfo := ClusterInfo{
        ServiceType: clusterValues[0],
        ProjectId: clusterValues[1],
        Region: clusterValues[2],
        ClusterName: clusterValues[3],
    }

    return &clusterInfo
}
