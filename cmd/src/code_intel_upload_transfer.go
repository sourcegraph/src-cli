package main

import (
	"context"
	"os"

	"github.com/sourcegraph/sourcegraph/lib/codeintel/upload"
	"github.com/sourcegraph/sourcegraph/lib/output"
)

func UploadUncompressedIndex(ctx context.Context, filename string, httpClient upload.Client, opts upload.UploadOptions) (int, error) {
	originalReader, originalSize, err := openFileAndGetSize(filename)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = originalReader.Close()
	}()

	bars := []output.ProgressBar{{Label: "Compressing", Max: 1.0}}
	progress, _, cleanup := logProgress(
		opts.Output,
		bars,
		"Index compressed",
		"Failed to compress index",
	)

	compressedFile, err := compressReaderToDisk(originalReader, originalSize, progress)
	if err != nil {
		cleanup(err)
		return 0, err
	}
	defer func() {
		_ = os.Remove(compressedFile)
	}()

	compressedSize, err := getFileSize(compressedFile)
	if err != nil {
		cleanup(err)
		return 0, err
	}

	cleanup(nil)

	if opts.Output != nil {
		opts.Output.WriteLine(output.Linef(
			output.EmojiLightbulb,
			output.StyleItalic,
			"Indexed compressed (%.2fMB -> %.2fMB).",
			float64(originalSize)/1000/1000,
			float64(compressedSize)/1000/1000,
		))
	}

	return UploadCompressedIndex(ctx, compressedFile, httpClient, opts, originalSize)
}

func UploadCompressedIndex(ctx context.Context, compressedFile string, httpClient upload.Client, opts upload.UploadOptions, uncompressedSize int64) (int, error) {
	compressedReader, compressedSize, err := openFileAndGetSize(compressedFile)
	if err != nil {
		// cleanup(err)
		return 0, err
	}
	defer func() {
		_ = compressedReader.Close()
	}()

	if compressedSize <= opts.MaxPayloadSizeBytes {
		return uploadIndex(ctx, httpClient, opts, compressedReader, compressedSize, uncompressedSize)
	}

	return uploadMultipartIndex(ctx, httpClient, opts, compressedReader, compressedSize, uncompressedSize)
}

func getFileSize(filename string) (int64, error) {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return 1, err
	}

	return fileInfo.Size(), nil
}
