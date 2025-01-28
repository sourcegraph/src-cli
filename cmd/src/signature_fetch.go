package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/output"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const signaturePublicKey = "https://storage.googleapis.com/sourcegraph-release-sboms/keys/cosign_keyring-cosign_image_signing_key-1.pub"

func init() {
	usage := `
'src signature verify' verifies signatures for the given release version of Sourcegraph.

Usage:

    src signature verify -v <version>

Examples:

    $ src signature verify -v 5.11.4013                 # Verify all signatures for the 5.11.4013 release

    $ src signature verify -v 6.0.0 -d /tmp/signatures  # Verify all signatures for the 6.0.0 release and write verified image digests under /tmp/signatures
`

	flagSet := flag.NewFlagSet("verify", flag.ExitOnError)
	versionFlag := flagSet.String("v", "", "The version of Sourcegraph to verify signatures for.")
	outputDirFlag := flagSet.String("d", "", "The directory to store verified image digests in.")
	insecureIgnoreTransparencyLogFlag := flagSet.Bool("insecure-ignore-tlog", false, "Disable transparency log verification. Defaults to false.")

	handler := func(args []string) error {
		c := cosignConfig{
			publicKey:       signaturePublicKey,
			internalRelease: false,
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

		if outputDirFlag != nil && *outputDirFlag != "" {
			c.outputDir = getOutputDir(*outputDirFlag, c.version)
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

		out.Writef("Verifying signatures for all %d images in the Sourcegraph %s release...\n", len(images), c.version)

		if c.insecureIgnoreTransparencyLog {
			out.WriteLine(output.Line("⚠️", output.StyleWarning, "WARNING: Transparency log verification is disabled, increasing the risk that images may have been tampered with."))
			out.WriteLine(output.Line("️", output.StyleWarning, "         This setting should only be used for testing or under explicit instruction from Sourcegraph.\n"))
		}

		var successCount, failureCount int
		var verifiedDigests []string
		for _, image := range images {
			stopSpinner := make(chan bool)
			go spinner(image, stopSpinner)

			verifiedDigest, err := c.verifySignatureForImageVersion(image, c.version)
			verifiedDigests = append(verifiedDigests, image+"@"+verifiedDigest)

			stopSpinner <- true

			if err != nil {
				out.WriteLine(output.Line(output.EmojiFailure, output.StyleWarning,
					fmt.Sprintf("\r%s: error verifying signature:\n    %v", image, err)))
				failureCount += 1
			} else {
				out.WriteLine(output.Line("\r\u2705", output.StyleSuccess, image+"@"+verifiedDigest))
				successCount += 1
			}
		}

		out.Write("")
		if successCount > 0 && failureCount > 0 {
			out.WriteLine(output.Line("🟠", output.StyleOrange, fmt.Sprintf("Verified signatures and digests for %d images, but failed to verify signatures for %d images", successCount, failureCount)))
		} else if successCount > 0 && failureCount == 0 {
			out.WriteLine(output.Line("🟢", output.StyleSuccess, fmt.Sprintf("Verified signatures and digests for %d images", successCount)))
		} else {
			out.WriteLine(output.Line("🔴", output.StyleWarning, "Failed to verify signatures for any images"))

		}

		if c.outputDir != "" {
			if err = c.writeVerifiedDigests(verifiedDigests); err != nil {
				out.WriteLine(output.Line("🔴", output.StyleWarning, err.Error()))
				return cmderrors.ExitCode1
			}
			out.Writef("\nVerified digests have been written to `%s`.\n", c.getOutputFilepath())
		}
		out.WriteLine(output.Linef("", output.StyleBold, "Your Sourcegraph deployment may not use all of these images. Please check your deployment to confirm which images are used.\n"))

		if failureCount > 0 || successCount == 0 {
			return cmderrors.ExitCode1
		}

		return nil
	}

	signatureCommands = append(signatureCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src signature %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func (c cosignConfig) verifySignatureForImageVersion(image string, version string) (string, error) {
	digest, err := getImageDigest(image, version)
	if err != nil {
		return "", err
	}

	err = c.verifySignatureForImageHash(image, digest)
	if err != nil {
		return "", err
	}

	// Only return the digest if the signature is verified
	return digest, nil
}

func (c cosignConfig) verifySignatureForImageHash(image string, hash string) error {
	tempDir, err := os.MkdirTemp("", "signature-")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cosignArgs := []string{
		"verify",
		"--key", c.publicKey,
		fmt.Sprintf("%s@%s", image, hash),
	}

	if c.insecureIgnoreTransparencyLog {
		cosignArgs = append(cosignArgs, "--insecure-ignore-tlog")
	}

	cmd := exec.Command("cosign", cosignArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Signature verification failed: %w\nOutput: %s", err, output)
	}

	return nil
}

func (c cosignConfig) writeVerifiedDigests(verifiedDigests []string) error {
	// Create the output file
	outputFile := c.getOutputFilepath()
	err := os.MkdirAll(c.outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write the verified digests to the file, one per line
	err = os.WriteFile(outputFile, []byte(strings.Join(verifiedDigests, "\n")+"\n"), 0644)
	if err != nil {
		return fmt.Errorf("failed to write verified digests to file: %w", err)
	}

	return nil
}

func (c cosignConfig) getOutputFilepath() string {
	return filepath.Join(c.outputDir, "verified-digests.txt")
}
