package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"cloud.google.com/go/storage"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/sourcegraph/lib/group"
	"github.com/sourcegraph/sourcegraph/lib/output"
	"google.golang.org/api/option"

	"github.com/sourcegraph/src-cli/internal/pgdump"
)

const srcSnapshotDir = "./src-snapshot"

var snapshotSummaryPath = path.Join(srcSnapshotDir, "summary.json")

// https://pkg.go.dev/cloud.google.com/go/storage#section-readme

func init() {
	usage := `'src snapshot upload' uploads snapshot contents.

USAGE
	src
`
	flagSet := flag.NewFlagSet("upload", flag.ExitOnError)
	bucketName := flagSet.String("bucket", "", "destination Cloud Storage bucket name")
	credentialsPath := flagSet.String("credentials", "", "JSON credentials file for Google Cloud service account")

	snapshotCommands = append(snapshotCommands, &command{
		flagSet: flagSet,
		handler: func(args []string) error {
			if err := flagSet.Parse(args); err != nil {
				return err
			}

			if *bucketName == "" {
				return errors.New("-bucket required")
			}
			if *credentialsPath == "" {
				return errors.New("-credentials required")
			}

			out := output.NewOutput(flagSet.Output(), output.OutputOpts{Verbose: *verbose})
			ctx := context.Background()
			c, err := storage.NewClient(ctx, option.WithCredentialsFile(*credentialsPath))
			if err != nil {
				return errors.Wrap(err, "create Cloud Storage client")
			}

			type upload struct {
				file *os.File
				stat os.FileInfo
			}
			var (
				uploads      []upload             // index aligned with progressBars
				progressBars []output.ProgressBar // index aligned with uploads
			)

			// Open snapshot summary
			if f, err := os.Open(snapshotSummaryPath); err != nil {
				return errors.Wrap(err, "failed to open snapshot summary - generate one with 'src snapshot summary'")
			} else {
				stat, err := f.Stat()
				if err != nil {
					return errors.Wrap(err, "get file size")
				}
				uploads = append(uploads, upload{
					file: f,
					stat: stat,
				})
				progressBars = append(progressBars, output.ProgressBar{
					Label: stat.Name(),
					Max:   float64(stat.Size()),
				})
			}

			// Open database dumps
			for _, o := range pgdump.Outputs(srcSnapshotDir, pgdump.Targets{}) {
				if f, err := os.Open(o.Output); err != nil {
					return errors.Wrap(err, "failed to database dump - generate one with 'src snapshot databases'")
				} else {
					stat, err := f.Stat()
					if err != nil {
						return errors.Wrap(err, "get file size")
					}
					uploads = append(uploads, upload{
						file: f,
						stat: stat,
					})
					progressBars = append(progressBars, output.ProgressBar{
						Label: stat.Name(),
						Max:   float64(stat.Size()),
					})
				}
			}

			// Start uploads
			progress := out.Progress(progressBars, nil)
			progress.WriteLine(output.Emoji(output.EmojiHourglass, "Starting uploads..."))
			bucket := c.Bucket(*bucketName)
			g := group.New().WithErrors().WithContext(ctx)
			for i, u := range uploads {
				i := i
				u := u
				g.Go(func(ctx context.Context) error {
					progressFn := func(p int64) { progress.SetValue(i, float64(p)) }

					if err := copyToBucket(ctx, u.file, u.stat, bucket, progressFn); err != nil {
						return errors.Wrap(err, u.stat.Name())
					}

					return nil
				})
			}

			// Finalize
			errs := g.Wait()
			progress.Complete()
			if errs != nil {
				out.WriteLine(output.Line(output.EmojiFailure, output.StyleFailure, "Some snapshot contents failed to upload."))
				return errs
			}

			out.WriteLine(output.Emoji(output.EmojiSuccess, "Summary contents uploaded!"))
			return nil
		},
		usageFunc: func() { fmt.Fprint(flag.CommandLine.Output(), usage) },
	})
}

func copyToBucket(ctx context.Context, src io.Reader, stat fs.FileInfo, dst *storage.BucketHandle, progressFn func(int64)) error {
	writer := dst.Object(stat.Name()).NewWriter(ctx)
	writer.ProgressFunc = progressFn
	defer writer.Close()

	// io.Copy is the best way to copy from a reader to writer in Go, and storage.Writer
	// has its own chunking mechanisms internally.
	written, err := io.Copy(writer, src)
	if err != nil {
		return err
	}

	// Progress is not called on completion, so we call it manually after io.Copy is done
	progressFn(written)

	// Validate we have sent all data
	size := stat.Size()
	if written != size {
		return errors.Newf("expected to write %d bytes, but actually wrote %d bytes",
			size, written)
	}

	return nil
}
