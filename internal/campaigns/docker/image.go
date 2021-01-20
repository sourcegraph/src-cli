package docker

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// Image represents a Docker image, hopefully stored in the local cache.
type Image struct {
	name string

	// There are lots of once fields below: basically, we're going to try fairly
	// hard to prevent performing the same operations on the same image over and
	// over, since some of them are expensive.

	digest     string
	digestOnce sync.Once

	ensureErr  error
	ensureOnce sync.Once

	uidGid     UIDGID
	uidGidOnce sync.Once
}

// UIDGID represents a UID:GID pair.
type UIDGID struct {
	UID int
	GID int
}

// Digest gets and returns the content digest for the image. Note that this is
// different from the "distribution digest" (which is what you can use to
// specify an image to `docker run`, as in `my/image@sha256:xxx`). We need to
// use the content digest because the distribution digest is only computed for
// images that have been pulled from or pushed to a registry. See
// https://windsock.io/explaining-docker-image-ids/ under "A Final Twist" for a
// good explanation.
func (image *Image) Digest(ctx context.Context) (string, error) {
	var err error
	image.digestOnce.Do(func() {
		image.digest, err = func() (string, error) {
			if err := image.Ensure(ctx); err != nil {
				return "", err
			}

			// TODO!(sqs): is image id the right thing to use here? it is NOT
			// the digest. but the digest is not calculated for all images
			// (unless they are pulled/pushed from/to a registry), see
			// https://github.com/moby/moby/issues/32016.
			out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", "--", image.name).CombinedOutput()
			if err != nil {
				return "", errors.Wrapf(err, "inspecting docker image: %s", string(bytes.TrimSpace(out)))
			}
			id := string(bytes.TrimSpace(out))
			if id == "" {
				return "", errors.Errorf("unexpected empty docker image content ID for %q", image)
			}
			return id, nil
		}()
	})

	if err != nil {
		return "", err
	}
	return image.digest, nil
}

// Ensure ensures that the image has been pulled by Docker. Note that it does
// not attempt to pull a newer version of the image if it exists locally.
func (image *Image) Ensure(ctx context.Context) error {
	image.ensureOnce.Do(func() {
		image.ensureErr = func() error {
			// docker image inspect will return a non-zero exit code if the image and
			// tag don't exist locally, regardless of the format.
			if err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "1", image.name).Run(); err != nil {
				// Let's try pulling the image.
				if err := exec.CommandContext(ctx, "docker", "image", "pull", image.name).Run(); err != nil {
					return errors.Wrap(err, "pulling image")
				}
			}

			return nil
		}()
	})

	return image.ensureErr
}

// UIDGID returns the user and group the container is configured to run as.
func (image *Image) UIDGID(ctx context.Context) (UIDGID, error) {
	var err error
	image.uidGidOnce.Do(func() {
		image.uidGid, err = func() (UIDGID, error) {
			stdout := new(bytes.Buffer)

			// Digest also implicitly means Ensure has been called.
			digest, err := image.Digest(ctx)
			if err != nil {
				return UIDGID{}, errors.Wrap(err, "getting digest")
			}

			args := []string{
				"run",
				"--rm",
				"--entrypoint", "/bin/sh",
				digest,
				"-c", "id -u; id -g",
			}
			cmd := exec.CommandContext(ctx, "docker", args...)
			cmd.Stdout = stdout

			if err := cmd.Run(); err != nil {
				return UIDGID{}, errors.Wrap(err, "running id")
			}

			// POSIX specifies the output of `id -u` as the effective UID,
			// terminated by a newline. `id -g` is the same, just for the GID.
			raw := strings.TrimSpace(stdout.String())
			lines := strings.Split(raw, "\n")
			if len(lines) < 2 {
				// There's an id command on the path, but it's not returning
				// POSIX compliant output.
				return UIDGID{}, errors.Wrap(err, "invalid id output")
			}

			uid, err := strconv.Atoi(lines[0])
			if err != nil {
				return UIDGID{}, errors.Wrap(err, "malformed uid")
			}

			gid, err := strconv.Atoi(lines[1])
			if err != nil {
				return UIDGID{}, errors.Wrap(err, "malformed gid")
			}

			return UIDGID{UID: uid, GID: gid}, nil
		}()
	})

	return image.uidGid, err
}
