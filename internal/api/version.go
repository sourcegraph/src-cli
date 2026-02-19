package api

import "context"

// GetSourcegraphVersion queries the Sourcegraph instance for its product version.
func GetSourcegraphVersion(ctx context.Context, client Client) (string, error) {
	var result struct {
		Site struct {
			ProductVersion string
		}
	}
	ok, err := client.NewQuery(`query { site { productVersion } }`).Do(ctx, &result)
	if err != nil || !ok {
		return "", err
	}
	return result.Site.ProductVersion, nil
}
