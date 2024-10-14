package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/grafana/regexp"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/lib/output"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

type sbomConfig struct {
	publicKey                     string
	outputDir                     string
	version                       string
	internalRelease               bool
	insecureIgnoreTransparencyLog bool
}

const publicKey = "https://storage.googleapis.com/sourcegraph-release-sboms/keys/cosign_keyring-cosign-1.pub"
const imageListBaseURL = "https://storage.googleapis.com/sourcegraph-release-sboms"
const imageListFilename = "release-image-list.txt"

func init() {
	usage := `
'src sbom fetch' fetches and verifies SBOMs for the given release version of Sourcegraph.

Usage:

    src sbom fetch -v <version>

Examples:

    $ src sbom fetch -v 5.8.0                            # Fetch all SBOMs for the 5.8.0 release

    $ src sbom fetch -v 5.8.123 -internal -d /tmp/sboms  # Fetch all SBOMs for the internal 5.8.123 release and store them in /tmp/sboms
`

	flagSet := flag.NewFlagSet("fetch", flag.ExitOnError)
	versionFlag := flagSet.String("v", "", "The version of Sourcegraph to fetch SBOMs for.")
	outputDirFlag := flagSet.String("d", "sourcegraph-sboms", "The directory to store validated SBOMs in.")
	internalReleaseFlag := flagSet.Bool("internal", false, "Fetch SBOMs for an internal release. Defaults to false.")
	insecureIgnoreTransparencyLogFlag := flagSet.Bool("insecure-ignore-tlog", false, "Disable transparency log verification. Defaults to false.")

	handler := func(args []string) error {
		c := sbomConfig{
			publicKey: publicKey,
		}

		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if len(flagSet.Args()) != 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}

		if versionFlag == nil || *versionFlag == "" {
			return cmderrors.Usage("version is required")
		}
		c.version = *versionFlag

		if outputDirFlag == nil || *outputDirFlag == "" {
			return cmderrors.Usage("output directory is required")
		}
		c.outputDir = getOutputDir(*outputDirFlag, *versionFlag)

		if internalReleaseFlag == nil || !*internalReleaseFlag {
			c.internalRelease = false
		} else {
			c.internalRelease = true
		}

		if insecureIgnoreTransparencyLogFlag != nil && *insecureIgnoreTransparencyLogFlag {
			c.insecureIgnoreTransparencyLog = true
		}

		out := output.NewOutput(flagSet.Output(), output.OutputOpts{Verbose: *verbose})

		if err := verifyCosign(); err != nil {
			return cmderrors.ExitCode(1, err)
		}

		images, err := c.getImageList()
		if err != nil {
			return err
		}

		out.Writef("Fetching SBOMs and validating signatures for all %d images in the Sourcegraph %s release...\n", len(images), c.version)

		if c.insecureIgnoreTransparencyLog {
			out.WriteLine(output.Line("⚠️", output.StyleWarning, "WARNING: Transparency log verification is disabled, increasing the risk that SBOMs may have been tampered with."))
			out.WriteLine(output.Line("️", output.StyleWarning, "         This setting should only be used for testing or under explicit instruction from Sourcegraph.\n"))
		}

		var successCount, failureCount int
		for _, image := range images {
			stopSpinner := make(chan bool)
			go spinner(image, stopSpinner)

			_, err = c.getSBOMForImageVersion(image, c.version)

			stopSpinner <- true

			if err != nil {
				out.WriteLine(output.Line(output.EmojiFailure, output.StyleWarning,
					fmt.Sprintf("\r%s: error fetching and validating SBOM:\n    %v", image, err)))
				failureCount += 1
			} else {
				out.WriteLine(output.Line("\r\u2705", output.StyleSuccess, image))
				successCount += 1
			}
		}

		out.Write("")
		if failureCount == 0 && successCount == 0 {
			out.WriteLine(output.Line("🔴", output.StyleWarning, "Failed to fetch SBOMs for any images"))
		}
		if failureCount > 0 {
			out.WriteLine(output.Line("🟠", output.StyleOrange, fmt.Sprintf("Fetched verified SBOMs for %d images, but failed to fetch SBOMs for %d images", successCount, failureCount)))
		} else if successCount > 0 {
			out.WriteLine(output.Line("🟢", output.StyleSuccess, fmt.Sprintf("Fetched verified SBOMs for %d images", successCount)))
		}

		out.Writef("\nFetched and validated SBOMs have been written to `%s`.\n", c.outputDir)
		out.WriteLine(output.Linef("", output.StyleBold, "Your Sourcegraph deployment may not use all of these images. Please check your deployment to confirm which images are used.\n"))

		if failureCount > 0 || successCount == 0 {
			return cmderrors.ExitCode1
		}

		return nil
	}

	sbomCommands = append(sbomCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src sbom %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func (c sbomConfig) getSBOMForImageVersion(image string, version string) (string, error) {
	hash, err := getImageDigest(image, version)
	if err != nil {
		return "", err
	}

	sbom, err := c.getSBOMForImageHash(image, hash)
	if err != nil {
		return "", err
	}

	return sbom, nil
}

func verifyCosign() error {
	_, err := exec.LookPath("cosign")
	if err != nil {
		return errors.New("SBOM verification requires 'cosign' to be installed and available in $PATH. See https://docs.sigstore.dev/cosign/system_config/installation/")
	}
	return nil
}

func (c sbomConfig) getImageList() ([]string, error) {
	imageReleaseListURL := c.getImageReleaseListURL()

	resp, err := http.Get(imageReleaseListURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {

		// Compare version number against a regex that matches versions up to and including 5.8.0
		versionRegex := regexp.MustCompile(`^v?[0-5]\.([0-7]\.[0-9]+|8\.0)$`)
		if versionRegex.MatchString(c.version) {
			return nil, fmt.Errorf("unsupported version %s: SBOMs are only available for Sourcegraph releases after 5.8.0", c.version)
		}
		return nil, fmt.Errorf("failed to fetch list of images - check that %s is a valid Sourcegraph release: HTTP status %d", c.version, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	var images []string
	for scanner.Scan() {
		image := strings.TrimSpace(scanner.Text())
		if image != "" {
			// Strip off a version suffix if present
			parts := strings.SplitN(image, ":", 2)
			images = append(images, parts[0])
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading image list: %w", err)
	}

	return images, nil
}

func (c sbomConfig) getSBOMForImageHash(image string, hash string) (string, error) {
	tempDir, err := os.MkdirTemp("", "sbom-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	outputFile := filepath.Join(tempDir, "attestation.json")

	cosignArgs := []string{
		"verify-attestation",
		"--key", publicKey,
		"--type", "cyclonedx",
		fmt.Sprintf("%s@%s", image, hash),
		"--output-file", outputFile,
	}

	if c.insecureIgnoreTransparencyLog {
		cosignArgs = append(cosignArgs, "--insecure-ignore-tlog")
	}

	cmd := exec.Command("cosign", cosignArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("SBOM fetching or validation failed: %w\nOutput: %s", err, output)
	}

	attestation, err := os.ReadFile(outputFile)
	if err != nil {
		return "", fmt.Errorf("failed to read SBOM file: %w", err)
	}

	sbom, err := extractSBOM(attestation)
	if err != nil {
		return "", fmt.Errorf("failed to extract SBOM from attestation: %w", err)
	}

	c.storeSBOM(sbom, image)

	return sbom, nil
}

type attestation struct {
	PayloadType   string `json:"payloadType"`
	Base64Payload string `json:"payload"`
}

func extractSBOM(attestationBytes []byte) (string, error) {
	var a attestation
	if err := json.Unmarshal(attestationBytes, &a); err != nil {
		return "", fmt.Errorf("failed to unmarshal attestation: %w", err)
	}

	if a.PayloadType != "application/vnd.in-toto+json" {
		return "", fmt.Errorf("unexpected payload type: %s", a.PayloadType)
	}

	decodedPayload, err := base64.StdEncoding.DecodeString(a.Base64Payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode payload: %w", err)
	}

	return string(decodedPayload), nil
}

func (c sbomConfig) storeSBOM(sbom string, image string) error {
	// Make the image name safe for use as a filename
	safeImageName := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, image)

	// Create the output file path
	outputFile := filepath.Join(c.outputDir, safeImageName+".json")

	// Ensure the output directory exists
	if err := os.MkdirAll(c.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write the SBOM to the file
	if err := os.WriteFile(outputFile, []byte(sbom), 0644); err != nil {
		return fmt.Errorf("failed to write SBOM file: %w", err)
	}

	return nil
}

func getOutputDir(parentDir, version string) string {
	return path.Join(parentDir, "sourcegraph-"+version)
}

// getImageReleaseListURL returns the URL for the list of images in a release, based on the version and whether it's an internal release.
func (c *sbomConfig) getImageReleaseListURL() string {
	if c.internalRelease {
		return fmt.Sprintf("%s/release-internal/%s/%s", imageListBaseURL, c.version, imageListFilename)
	} else {
		return fmt.Sprintf("%s/release/%s/%s", imageListBaseURL, c.version, imageListFilename)
	}
}
