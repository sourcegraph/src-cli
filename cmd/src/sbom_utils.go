package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path"
	"strings"
	"time"
)

// TokenResponse represents the JSON response from dockerHub's token service
type dockerHubTokenResponse struct {
	Token string `json:"token"`
}

// getImageDigest returns the sha256 hash for the given image and tag
// It supports multiple registries
func getImageDigest(image string, tag string) (string, error) {
	if strings.HasPrefix(image, "sourcegraph/") {
		return getImageDigestDockerHub(image, tag)
	} else if strings.HasPrefix(image, "us-central1-docker.pkg.dev/") {
		return getImageDigestGcloud(image, tag)
	} else {
		return "", fmt.Errorf("unsupported image registry: %s", image)
	}
}

//
// Implement functionality for Docker Hub

// getImageDigestDockerHub returns the sha256 digest for the given image and tag from DockerHub
func getImageDigestDockerHub(image string, tag string) (string, error) {
	// Construct the DockerHub manifest URL
	url := fmt.Sprintf("https://registry-1.docker.io/v2/%s/manifests/%s", image, tag)

	token, err := getDockerHubAuthToken(image)
	if err != nil {
		return "", err
	}

	// Create a new HTTP request with the authorization header
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")

	// Make the HTTP request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch image manifest: %v", err)
	}
	defer resp.Body.Close()

	// Check for a successful response
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get manifest - check %s is a valid Sourcegraph release, status code: %d", tag, resp.StatusCode)
	}

	// Get the image digest from the `Docker-Content-Digest` header
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("digest not found in response headers")
	}
	// Return the image's digest (hash)
	return digest, nil
}

// getDockerHubAuthToken returns an auth token with scope to pull the given image
// Note that the token has a short validity so it should be used immediately
func getDockerHubAuthToken(image string) (string, error) {
	// Set the DockerHub authentication URL
	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", image)

	// Create a new HTTP request
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %v", err)
	}
	defer resp.Body.Close()

	// Check if the response status is 200 OK
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token, status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// Unmarshal the JSON response
	var tokenResponse dockerHubTokenResponse
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("failed to parse token response: %v", err)
	}

	// Return the token
	return tokenResponse.Token, nil
}

//
// Implement functionality for GCP Artifact Registry

// getImageDigestGcloud fetches the OCI image manifest from GCP Artifact Registry and returns the image digest
func getImageDigestGcloud(image string, tag string) (string, error) {
	// Validate image path to ensure it's a valid GCP Artifact Registry image
	if !strings.HasPrefix(image, "us-central1-docker.pkg.dev/") {
		return "", fmt.Errorf("invalid image format: %s", image)
	}

	// Get the GCP access token
	token, err := getGcloudAccessToken()
	if err != nil {
		return "", fmt.Errorf("error getting access token: %v", err)
	}

	parts := strings.SplitN(image, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid image format: %s", image)
	}
	domain := parts[0]
	repositoryPath := parts[1]

	// Create the URL to fetch the manifest for the specific image and tag
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", domain, repositoryPath, tag)

	// Create a new HTTP GET request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Add the Authorization and Accept headers
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")

	// Perform the HTTP request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %v", err)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get manifest, status code: %d, response: %s", resp.StatusCode, string(body))
	}

	// Get the image digest from the `Docker-Content-Digest` header
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("digest not found in response headers")
	}

	return digest, nil
}

// getGcloudAccessToken runs 'gcloud auth print-access-token' and returns the access token
func getGcloudAccessToken() (string, error) {
	// Execute the gcloud command to get the access token
	cmd := exec.Command("gcloud", "auth", "print-access-token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve access token using `gcloud auth`. Ensure that gcloud is installed and you have authenticated: %v", err)
	}

	// Trim any extra whitespace or newlines
	token := strings.TrimSpace(string(out))
	return token, nil
}

var spinnerChars = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

func spinner(name string, stop chan bool) {
	i := 0
	for {
		select {
		case <-stop:
			return
		default:
			fmt.Printf("\r%s  %s", string(spinnerChars[i%len(spinnerChars)]), name)
			i++
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func getOutputDir(parentDir, version string) string {
	return path.Join(parentDir, "sourcegraph-"+version)
}

// sanitizeVersion removes any leading "v" from the version string
func sanitizeVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}
