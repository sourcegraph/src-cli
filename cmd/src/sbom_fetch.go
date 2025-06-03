package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sourcegraph/sourcegraph/lib/output"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const sbomPublicKey = "https://storage.googleapis.com/sourcegraph-release-sboms/keys/cosign_keyring-cosign-1.pub"

func init() {
	usage := `
'src sbom fetch' fetches and verifies SBOMs for the given release version of Sourcegraph.

Usage:

    src sbom fetch -v <version> [--image <image-patterns>] [--exclude-image <exclude-patterns>]

Examples:

    $ src sbom fetch -v 5.8.0                              # Fetch all SBOMs for the 5.8.0 release

    $ src sbom fetch -v 5.8.0 --image frontend             # Fetch SBOM only for the frontend image

    $ src sbom fetch -v 5.8.0 --image "redis*"             # Fetch SBOMs for all images with names beginning with 'redis'

    $ src sbom fetch -v 5.8.0 --image "frontend,redis*"    # Fetch SBOMs for frontend, and all redis images

    $ src sbom fetch -v 5.8.0 --exclude-image "sg,*redis*" # Fetch SBOMs for all images, except sg and redis

    $ src sbom fetch -v 5.8.0 --image "postgres*" --exclude-image "*exporter*" # Fetch SBOMs for all postgres images, except exporters

    $ src sbom fetch -v 5.8.123 -internal -d /tmp/sboms    # Fetch all SBOMs for the internal 5.8.123 release and store them in /tmp/sboms
`

	flagSet := flag.NewFlagSet("fetch", flag.ExitOnError)
	versionFlag := flagSet.String("v", "", "The version of Sourcegraph to fetch SBOMs for.")
	outputDirFlag := flagSet.String("d", "sourcegraph-sboms", "The directory to store validated SBOMs in.")
	internalReleaseFlag := flagSet.Bool("internal", false, "Fetch SBOMs for an internal release. Defaults to false.")
	insecureIgnoreTransparencyLogFlag := flagSet.Bool("insecure-ignore-tlog", false, "Disable transparency log verification. Defaults to false.")
	imageFlag := flagSet.String("image", "", "Filter list of image names, to only fetch SBOMs for Docker images with names matching these patterns. Supports literal names, like frontend, and glob patterns like '*postgres*'. Multiple patterns can be specified as a comma-separated list (e.g., 'frontend,*postgres-1?-*'). The 'sourcegraph/' prefix is optional. If not specified, SBOMs for all images are fetched.")
	excludeImageFlag := flagSet.String("exclude-image", "", "Exclude Docker images with names matching these patterns from being fetched. Supports the same formats as --image. Takes precedence over --image filters.")

	handler := func(args []string) error {
		c := cosignConfig{
			publicKey: sbomPublicKey,
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
		c.version = sanitizeVersion(*versionFlag)

		if outputDirFlag == nil || *outputDirFlag == "" {
			return cmderrors.Usage("output directory is required")
		}
		c.outputDir = getOutputDir(*outputDirFlag, c.version)

		if internalReleaseFlag == nil || !*internalReleaseFlag {
			c.internalRelease = false
		} else {
			c.internalRelease = true
		}

		if insecureIgnoreTransparencyLogFlag != nil && *insecureIgnoreTransparencyLogFlag {
			c.insecureIgnoreTransparencyLog = true
		}

		if imageFlag != nil && *imageFlag != "" {
			// Parse comma-separated patterns
			patterns := strings.Split(*imageFlag, ",")
			for i, pattern := range patterns {
				patterns[i] = strings.TrimSpace(pattern)
			}
			c.imageFilters = patterns
		}

		if excludeImageFlag != nil && *excludeImageFlag != "" {
			// Parse comma-separated exclude patterns
			patterns := strings.Split(*excludeImageFlag, ",")
			for i, pattern := range patterns {
				patterns[i] = strings.TrimSpace(pattern)
			}
			c.excludeImageFilters = patterns
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

func (c cosignConfig) getSBOMForImageVersion(image string, version string) (string, error) {
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

func (c cosignConfig) getSBOMForImageHash(image string, hash string) (string, error) {
	tempDir, err := os.MkdirTemp("", "sbom-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	outputFile := filepath.Join(tempDir, "attestation.json")

	cosignArgs := []string{
		"verify-attestation",
		"--key", c.publicKey,
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
	// Ensure we only use the first line - occasionally Cosign includes multiple lines
	lines := bytes.Split(attestationBytes, []byte("\n"))
	if len(lines) == 0 {
		return "", fmt.Errorf("attestation is empty")
	}

	var a attestation
	if err := json.Unmarshal(lines[0], &a); err != nil {
		return "", fmt.Errorf("failed to unmarshal attestation: %w", err)
	}

	if a.PayloadType != "application/vnd.in-toto+json" {
		return "", fmt.Errorf("unexpected payload type: %s", a.PayloadType)
	}

	decodedPayload, err := base64.StdEncoding.DecodeString(a.Base64Payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode payload: %w", err)
	}

	// Unmarshal the decoded payload to extract predicate
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(decodedPayload, &payload); err != nil {
		return "", fmt.Errorf("failed to unmarshal decoded payload: %w", err)
	}

	// Extract just the predicate field
	predicate, ok := payload["predicate"]
	if !ok {
		return "", fmt.Errorf("no predicate field found in payload")
	}

	return string(predicate), nil
}

func (c cosignConfig) storeSBOM(sbom string, image string) error {
	// Make the image name safe for use as a filename
	safeImageName := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, image)

	// Create the output file path
	outputFile := filepath.Join(c.outputDir, safeImageName+".cdx.json")

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
