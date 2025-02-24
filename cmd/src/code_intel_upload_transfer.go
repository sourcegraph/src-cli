package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/sourcegraph/conc/pool"
	"github.com/sourcegraph/sourcegraph/lib/codeintel/upload"
	"github.com/sourcegraph/sourcegraph/lib/errors"
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

	compressedReader, compressedSize, err := openFileAndGetSize(compressedFile)
	if err != nil {
		cleanup(err)
		return 0, err
	}
	defer func() {
		_ = compressedReader.Close()
	}()

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

	if compressedSize <= opts.MaxPayloadSizeBytes {
		return uploadIndex(ctx, httpClient, opts, compressedReader, compressedSize, &originalSize)
	}

	return uploadMultipartIndex(ctx, httpClient, opts, compressedReader, compressedSize, &originalSize)
}

func UploadCompressedIndex(ctx context.Context, compressedFile string, httpClient upload.Client, opts upload.UploadOptions) (int, error) {
	compressedReader, compressedSize, err := openFileAndGetSize(compressedFile)
	if err != nil {
		// cleanup(err)
		return 0, err
	}
	defer func() {
		_ = compressedReader.Close()
	}()

	if compressedSize <= opts.MaxPayloadSizeBytes {
		return uploadIndex(ctx, httpClient, opts, compressedReader, compressedSize, nil)
	}

	return uploadMultipartIndex(ctx, httpClient, opts, compressedReader, compressedSize, nil)
}

/*
	NOTE:

	All the definitions below have been vendored in from the Sourcegraph repository public snapshot at
	this commit: https://github.com/sourcegraph/sourcegraph-public-snapshot/commit/1af563b61442c255af7b07a526efd71b3b0bad0d

	All necessary definitions were vendored in without changes until the code built successfully.
*/

// uploadIndex uploads the index file described by the given options to a Sourcegraph
// instance via a single HTTP POST request. The identifier of the upload is returned
// after a successful upload.
func uploadIndex(ctx context.Context, httpClient upload.Client, opts upload.UploadOptions, r io.ReaderAt, readerLen int64, uncompressedSize *int64) (id int, err error) {
	bars := []output.ProgressBar{{Label: "Upload", Max: 1.0}}
	progress, retry, complete := logProgress(
		opts.Output,
		bars,
		"Index uploaded",
		"Failed to upload index file",
	)
	defer func() { complete(err) }()

	// Create a section reader that can reset our reader view for retries
	reader := io.NewSectionReader(r, 0, readerLen)

	requestOptions := uploadRequestOptions{
		UploadOptions: opts,
		Target:        &id,
	}
	if uncompressedSize != nil {
		requestOptions.UncompressedSize = *uncompressedSize
	}
	err = uploadIndexFile(ctx, httpClient, opts, reader, readerLen, requestOptions, progress, retry, 0, 1)

	if progress != nil {
		// Mark complete in case we debounced our last updates
		progress.SetValue(0, 1)
	}

	return id, err
}

// uploadIndexFile uploads the contents available via the given reader to a
// Sourcegraph instance with the given request options.i
func uploadIndexFile(ctx context.Context, httpClient upload.Client, uploadOptions upload.UploadOptions, reader io.ReadSeeker, readerLen int64, requestOptions uploadRequestOptions, progress output.Progress, retry onRetryLogFn, barIndex int, numParts int) error {
	retrier := makeRetry(uploadOptions.MaxRetries, uploadOptions.RetryInterval)

	return retrier(func(attempt int) (_ bool, err error) {
		defer func() {
			if err != nil && !errors.Is(err, ctx.Err()) && progress != nil {
				progress.SetValue(barIndex, 0)
			}
		}()

		if attempt != 0 {
			suffix := ""
			if numParts != 1 {
				suffix = fmt.Sprintf(" %d of %d", barIndex+1, numParts)
			}

			if progress != nil {
				progress.SetValue(barIndex, 0)
			}
			progress = retry(fmt.Sprintf("Failed to upload index file%s (will retry; attempt #%d)", suffix, attempt))
		}

		// Create fresh reader on each attempt
		reader.Seek(0, io.SeekStart)

		// Report upload progress as writes occur
		requestOptions.Payload = newProgressCallbackReader(reader, readerLen, progress, barIndex)

		// Perform upload
		return performUploadRequest(ctx, httpClient, requestOptions)
	})
}

// uploadMultipartIndex uploads the index file described by the given options to a
// Sourcegraph instance over multiple HTTP POST requests. The identifier of the upload
// is returned after a successful upload.
func uploadMultipartIndex(ctx context.Context, httpClient upload.Client, opts upload.UploadOptions, r io.ReaderAt, readerLen int64, uncompressedSize *int64) (_ int, err error) {
	// Create a slice of section readers for upload part retries.
	// This allows us to both read concurrently from the same reader,
	// but also retry reads from arbitrary offsets.
	readers := splitReader(r, readerLen, opts.MaxPayloadSizeBytes)

	// Perform initial request that gives us our upload identifier
	id, err := uploadMultipartIndexInit(ctx, httpClient, opts, len(readers), uncompressedSize)
	if err != nil {
		return 0, err
	}

	// Upload each payload of the multipart index
	if err := uploadMultipartIndexParts(ctx, httpClient, opts, readers, id, readerLen); err != nil {
		return 0, err
	}

	// Finalize the upload and mark it as ready for processing
	if err := uploadMultipartIndexFinalize(ctx, httpClient, opts, id); err != nil {
		return 0, err
	}

	return id, nil
}

// uploadMultipartIndexInit performs an initial request to prepare the backend to accept upload
// parts via additional HTTP requests. This upload will be in a pending state until all upload
// parts are received and the multipart upload is finalized, or until the record is deleted by
// a background process after an expiry period.
func uploadMultipartIndexInit(ctx context.Context, httpClient upload.Client, opts upload.UploadOptions, numParts int, uncompressedSize *int64) (id int, err error) {
	retry, complete := logPending(
		opts.Output,
		"Preparing multipart upload",
		"Prepared multipart upload",
		"Failed to prepare multipart upload",
	)
	defer func() { complete(err) }()

	err = makeRetry(opts.MaxRetries, opts.RetryInterval)(func(attempt int) (bool, error) {
		if attempt != 0 {
			retry(fmt.Sprintf("Failed to prepare multipart upload (will retry; attempt #%d)", attempt))
		}

		options := uploadRequestOptions{
			UploadOptions: opts,
			Target:        &id,
			MultiPart:     true,
			NumParts:      numParts,
		}
		if uncompressedSize != nil {
			options.UncompressedSize = *uncompressedSize
		}

		return performUploadRequest(ctx, httpClient, options)
	})

	return id, err
}

// uploadMultipartIndexParts uploads the contents available via each of the given reader(s)
// to a Sourcegraph instance as part of the same multipart upload as indiciated
// by the given identifier.
func uploadMultipartIndexParts(ctx context.Context, httpClient upload.Client, opts upload.UploadOptions, readers []io.ReadSeeker, id int, readerLen int64) (err error) {
	var bars []output.ProgressBar
	for i := range readers {
		label := fmt.Sprintf("Upload part %d of %d", i+1, len(readers))
		bars = append(bars, output.ProgressBar{Label: label, Max: 1.0})
	}
	progress, retry, complete := logProgress(
		opts.Output,
		bars,
		"Index parts uploaded",
		"Failed to upload index parts",
	)
	defer func() { complete(err) }()

	pool := new(pool.ErrorPool).WithFirstError().WithContext(ctx)
	if opts.MaxConcurrency > 0 {
		pool.WithMaxGoroutines(opts.MaxConcurrency)
	}

	for i, reader := range readers {
		i, reader := i, reader

		pool.Go(func(ctx context.Context) error {
			// Determine size of this reader. If we're not the last reader in the slice,
			// then we're the maximum payload size. Otherwise, we're whatever is left.
			partReaderLen := opts.MaxPayloadSizeBytes
			if i == len(readers)-1 {
				partReaderLen = readerLen - int64(len(readers)-1)*opts.MaxPayloadSizeBytes
			}

			requestOptions := uploadRequestOptions{
				UploadOptions: opts,
				UploadID:      id,
				Index:         i,
			}

			if err := uploadIndexFile(ctx, httpClient, opts, reader, partReaderLen, requestOptions, progress, retry, i, len(readers)); err != nil {
				return err
			} else if progress != nil {
				// Mark complete in case we debounced our last updates
				progress.SetValue(i, 1)
			}
			return nil
		})
	}

	return pool.Wait()
}

// uploadMultipartIndexFinalize performs the request to stitch the uploaded parts together and
// mark it ready as processing in the backend.
func uploadMultipartIndexFinalize(ctx context.Context, httpClient upload.Client, opts upload.UploadOptions, id int) (err error) {
	retry, complete := logPending(
		opts.Output,
		"Finalizing multipart upload",
		"Finalized multipart upload",
		"Failed to finalize multipart upload",
	)
	defer func() { complete(err) }()

	return makeRetry(opts.MaxRetries, opts.RetryInterval)(func(attempt int) (bool, error) {
		if attempt != 0 {
			retry(fmt.Sprintf("Failed to finalize multipart upload (will retry; attempt #%d)", attempt))
		}

		return performUploadRequest(ctx, httpClient, uploadRequestOptions{
			UploadOptions: opts,
			UploadID:      id,
			Done:          true,
		})
	})
}

// splitReader returns a slice of read-seekers into the input ReaderAt, each of max size maxPayloadSize.
//
// The sequential concatenation of each reader produces the content of the original reader.
//
// Each reader is safe to use concurrently with others. The original reader should be closed when all produced
// readers are no longer active.
func splitReader(r io.ReaderAt, n, maxPayloadSize int64) (readers []io.ReadSeeker) {
	for offset := int64(0); offset < n; offset += maxPayloadSize {
		readers = append(readers, io.NewSectionReader(r, offset, maxPayloadSize))
	}

	return readers
}

// openFileAndGetSize returns an open file handle and the size on disk for the given filename.
func openFileAndGetSize(filename string) (*os.File, int64, error) {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return nil, 0, err
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, 0, err
	}

	return file, fileInfo.Size(), err
}

// logPending creates a pending object from the given output value and returns a retry function that
// can be called to print a message then reset the pending display, and a complete function that should
// be called once the work attached to this log call has completed. This complete function takes an error
// value that determines whether the success or failure message is displayed. If the given output value is
// nil then a no-op complete function is returned.
func logPending(out *output.Output, pendingMessage, successMessage, failureMessage string) (func(message string), func(error)) {
	if out == nil {
		return func(message string) {}, func(err error) {}
	}

	pending := out.Pending(output.Line("", output.StylePending, pendingMessage))

	retry := func(message string) {
		pending.Destroy()
		out.WriteLine(output.Line(output.EmojiFailure, output.StyleReset, message))
		pending = out.Pending(output.Line("", output.StylePending, pendingMessage))
	}

	complete := func(err error) {
		if err == nil {
			pending.Complete(output.Line(output.EmojiSuccess, output.StyleSuccess, successMessage))
		} else {
			pending.Complete(output.Line(output.EmojiFailure, output.StyleBold, failureMessage))
		}
	}

	return retry, complete
}

type onRetryLogFn func(message string) output.Progress

// logProgress creates and returns a progress from the given output value and bars configuration.
// This function also returns a retry function that can be called to print a message then reset the
// progress bar display, and a complete function that should be called once the work attached to
// this log call has completed. This complete function takes an error value that determines whether
// the success or failure message is displayed. If the given output value is nil then a no-op complete
// function is returned.
func logProgress(out *output.Output, bars []output.ProgressBar, successMessage, failureMessage string) (output.Progress, onRetryLogFn, func(error)) {
	if out == nil {
		return nil, func(message string) output.Progress { return nil }, func(err error) {}
	}

	var mu sync.Mutex
	progress := out.Progress(bars, nil)

	retry := func(message string) output.Progress {
		mu.Lock()
		defer mu.Unlock()

		progress.Destroy()
		out.WriteLine(output.Line(output.EmojiFailure, output.StyleReset, message))
		progress = out.Progress(bars, nil)
		return progress
	}

	complete := func(err error) {
		progress.Destroy()

		if err == nil {
			out.WriteLine(output.Line(output.EmojiSuccess, output.StyleSuccess, successMessage))
		} else {
			out.WriteLine(output.Line(output.EmojiFailure, output.StyleBold, failureMessage))
		}
	}

	return progress, retry, complete
}

type uploadRequestOptions struct {
	upload.UploadOptions

	Payload          io.Reader // Request payload
	Target           *int      // Pointer to upload id decoded from resp
	MultiPart        bool      // Whether the request is a multipart init
	NumParts         int       // The number of upload parts
	UncompressedSize int64     // The uncompressed size of the upload
	UploadID         int       // The multipart upload ID
	Index            int       // The index part being uploaded
	Done             bool      // Whether the request is a multipart finalize
}

// ErrUnauthorized occurs when the upload endpoint returns a 401 response.
var ErrUnauthorized = errors.New("unauthorized upload")

// performUploadRequest performs an HTTP POST to the upload endpoint. The query string of the request
// is constructed from the given request options and the body of the request is the unmodified reader.
// If target is a non-nil pointer, it will be assigned the value of the upload identifier present
// in the response body. This function returns an error as well as a boolean flag indicating if the
// function can be retried.
func performUploadRequest(ctx context.Context, httpClient upload.Client, opts uploadRequestOptions) (bool, error) {
	req, err := makeUploadRequest(opts)
	if err != nil {
		return false, err
	}

	resp, body, err := performRequest(ctx, req, httpClient, opts.OutputOptions.Logger)
	if err != nil {
		return false, err
	}

	return decodeUploadPayload(resp, body, opts.Target)
}

// makeUploadRequest creates an HTTP request to the upload endpoint described by the given arguments.
func makeUploadRequest(opts uploadRequestOptions) (*http.Request, error) {
	uploadURL, err := makeUploadURL(opts)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", uploadURL.String(), opts.Payload)
	if err != nil {
		return nil, err
	}
	if opts.UncompressedSize != 0 {
		req.Header.Set("X-Uncompressed-Size", strconv.Itoa(int(opts.UncompressedSize)))
	}
	if opts.SourcegraphInstanceOptions.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", opts.SourcegraphInstanceOptions.AccessToken))
	}

	for k, v := range opts.SourcegraphInstanceOptions.AdditionalHeaders {
		req.Header.Set(k, v)
	}

	return req, nil
}

// performRequest performs an HTTP request and returns the HTTP response as well as the entire
// body as a byte slice. If a logger is supplied, the request, response, and response body will
// be logged.
func performRequest(ctx context.Context, req *http.Request, httpClient upload.Client, logger upload.RequestLogger) (*http.Response, []byte, error) {
	started := time.Now()
	if logger != nil {
		logger.LogRequest(req)
	}

	resp, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if logger != nil {
		logger.LogResponse(req, resp, body, time.Since(started))
	}
	if err != nil {
		return nil, nil, err
	}

	return resp, body, nil
}

// decodeUploadPayload reads the given response to an upload request. If target is a non-nil pointer,
// it will be assigned the value of the upload identifier present in the response body. This function
// returns a boolean flag indicating if the function can be retried on failure (error-dependent).
func decodeUploadPayload(resp *http.Response, body []byte, target *int) (bool, error) {
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			return false, ErrUnauthorized
		}

		suffix := ""
		if !bytes.HasPrefix(bytes.TrimSpace(body), []byte{'<'}) {
			suffix = fmt.Sprintf(" (%s)", bytes.TrimSpace(body))
		}

		// Do not retry client errors
		return resp.StatusCode >= 500, errors.Errorf("unexpected status code: %d%s", resp.StatusCode, suffix)
	}

	if target == nil {
		// No target expected, skip decoding body
		return false, nil
	}

	var respPayload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &respPayload); err != nil {
		return false, errors.Errorf("unexpected response (%s)", err)
	}

	id, err := strconv.Atoi(respPayload.ID)
	if err != nil {
		return false, errors.Errorf("unexpected response (%s)", err)
	}

	*target = id
	return false, nil
}

// makeUploadURL creates a URL pointing to the configured Sourcegraph upload
// endpoint with the query string described by the given request options.
func makeUploadURL(opts uploadRequestOptions) (*url.URL, error) {
	qs := url.Values{}

	if opts.SourcegraphInstanceOptions.GitHubToken != "" {
		qs.Add("github_token", opts.SourcegraphInstanceOptions.GitHubToken)
	}
	if opts.SourcegraphInstanceOptions.GitLabToken != "" {
		qs.Add("gitlab_token", opts.SourcegraphInstanceOptions.GitLabToken)
	}
	if opts.UploadRecordOptions.Repo != "" {
		qs.Add("repository", opts.UploadRecordOptions.Repo)
	}
	if opts.UploadRecordOptions.Commit != "" {
		qs.Add("commit", opts.UploadRecordOptions.Commit)
	}
	if opts.UploadRecordOptions.Root != "" {
		qs.Add("root", opts.UploadRecordOptions.Root)
	}
	if opts.UploadRecordOptions.Indexer != "" {
		qs.Add("indexerName", opts.UploadRecordOptions.Indexer)
	}
	if opts.UploadRecordOptions.IndexerVersion != "" {
		qs.Add("indexerVersion", opts.UploadRecordOptions.IndexerVersion)
	}
	if opts.UploadRecordOptions.AssociatedIndexID != nil {
		qs.Add("associatedIndexId", formatInt(*opts.UploadRecordOptions.AssociatedIndexID))
	}
	if opts.MultiPart {
		qs.Add("multiPart", "true")
	}
	if opts.NumParts != 0 {
		qs.Add("numParts", formatInt(opts.NumParts))
	}
	if opts.UploadID != 0 {
		qs.Add("uploadId", formatInt(opts.UploadID))
	}
	if opts.UploadID != 0 && !opts.MultiPart && !opts.Done {
		// Do not set an index of zero unless we're uploading a part
		qs.Add("index", formatInt(opts.Index))
	}
	if opts.Done {
		qs.Add("done", "true")
	}

	path := opts.SourcegraphInstanceOptions.Path
	if path == "" {
		path = "/.api/lsif/upload"
	}

	parsedUrl, err := url.Parse(opts.SourcegraphInstanceOptions.SourcegraphURL + path)
	if err != nil {
		return nil, err
	}

	parsedUrl.RawQuery = qs.Encode()
	return parsedUrl, nil
}

func formatInt(v int) string {
	return strconv.FormatInt(int64(v), 10)
}

// RetryableFunc is a function that takes the invocation index and returns an error as well as a
// boolean-value flag indicating whether or not the error is considered retryable.
type RetryableFunc = func(attempt int) (bool, error)

// makeRetry returns a function that calls retry with the given max attempt and interval values.
func makeRetry(n int, interval time.Duration) func(f RetryableFunc) error {
	return func(f RetryableFunc) error {
		return retry(f, n, interval)
	}
}

// retry will re-invoke the given function until it returns a nil error value, the function returns
// a non-retryable error (as indicated by its boolean return value), or until the maximum number of
// retries have been attempted. All errors encountered will be returned.
func retry(f RetryableFunc, n int, interval time.Duration) (errs error) {
	for i := 0; i <= n; i++ {
		retry, err := f(i)

		errs = errors.CombineErrors(errs, err)

		if err == nil || !retry {
			break
		}

		time.Sleep(interval)
	}

	return errs
}

type progressCallbackReader struct {
	reader           io.Reader
	totalRead        int64
	progressCallback func(totalRead int64)
}

var debounceInterval = time.Millisecond * 50

// newProgressCallbackReader returns a modified version of the given reader that
// updates the value of a progress bar on each read. If progress is nil or n is
// zero, then the reader is returned unmodified.
//
// Calls to the progress bar update will be debounced so that two updates do not
// occur within 50ms of each other. This is to reduce flicker on the screen for
// massive writes, which make progress more quickly than the screen can redraw.
func newProgressCallbackReader(r io.Reader, readerLen int64, progress output.Progress, barIndex int) io.Reader {
	if progress == nil || readerLen == 0 {
		return r
	}

	var lastUpdated time.Time

	progressCallback := func(totalRead int64) {
		if debounceInterval <= time.Since(lastUpdated) {
			// Calculate progress through the reader; do not ever complete
			// as we wait for the HTTP request finish the remaining small
			// percentage.

			p := float64(totalRead) / float64(readerLen)
			if p >= 1 {
				p = 1 - 10e-3
			}

			lastUpdated = time.Now()
			progress.SetValue(barIndex, p)
		}
	}

	return &progressCallbackReader{reader: r, progressCallback: progressCallback}
}

func (r *progressCallbackReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.totalRead += int64(n)
	r.progressCallback(r.totalRead)
	return n, err
}

// compressReaderToDisk compresses and writes the content of the given reader to a temporary
// file and returns the file's path. If the given progress object is non-nil, then the progress's
// first bar will be updated with the percentage of bytes read on each read.
func compressReaderToDisk(r io.Reader, readerLen int64, progress output.Progress) (filename string, err error) {
	compressedFile, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := compressedFile.Close(); err != nil {
			err = errors.Append(err, closeErr)
		}
	}()

	gzipWriter := gzip.NewWriter(compressedFile)
	defer func() {
		if closeErr := gzipWriter.Close(); err != nil {
			err = errors.Append(err, closeErr)
		}
	}()

	if progress != nil {
		r = newProgressCallbackReader(r, readerLen, progress, 0)
	}
	if _, err := io.Copy(gzipWriter, r); err != nil {
		return "", nil
	}

	return compressedFile.Name(), nil
}
