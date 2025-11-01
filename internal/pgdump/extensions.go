package pgdump

import (
	"bufio"
	"bytes"
	"io"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// CommentOutInvalidLines will comment out lines in the customer's SQL database dump file
// which gcloud sql import errors out on
//
// It performs a partial copy of a SQL database dump from
// src to dst while commenting out the problematic lines.
// When it determines there are no more EXTENSIONs-related statements,
// it will return, resetting src to the position of the last contents written to dst.
//
// This is needed for import to Google Cloud Storage, which does not like many statements which pg_dump may insert
// For more details, see https://cloud.google.com/sql/docs/postgres/import-export/import-export-dmp
//
// Filtering requires reading entire lines into memory - this can be a very expensive
// operation, so when filtering is complete, the more efficient io.Copy should be used
// to perform the remainder of the copy from src to dst.
func CommentOutInvalidLines(dst io.Writer, src io.ReadSeeker, progressFn func(int64)) (int64, error) {
	var (
		reader = bufio.NewReader(src)

		// Position we have consumed up to
		// Tracked separately because bufio.Reader may have read ahead on src
		// This allows us to reset src later
		consumed int64

		// Number of bytes we have actually written to dst
		// It should always be returned
		written int64

		// Set to true when we start to hit lines which indicate that we may be finished filtering
		noMoreLinesToFilter bool

		filterEndMarkers = []string{
			"CREATE TABLE",
			"INSERT INTO",
		}

		linesToFilter = []string{

			"DROP DATABASE",
			"CREATE DATABASE",
			"COMMENT ON DATABASE",

			"DROP SCHEMA",
			"CREATE SCHEMA",
			"COMMENT ON SCHEMA",

			"DROP EXTENSION",
			"CREATE EXTENSION",
			"COMMENT ON EXTENSION",

			"SET transaction_timeout", // pg_dump v17, importing to Postgres 16

			"\\connect",
			// "\\restrict",
			// "\\unrestrict",
		}
	)

	for !noMoreLinesToFilter {

		// Read up to a line, keeping track of our position in src
		line, err := reader.ReadBytes('\n')
		consumed += int64(len(line))

		// If this function has read through the whole file,
		// then hand the last line
		if err == io.EOF {
			noMoreLinesToFilter = true

			// If the reader has found a different error,
			// then return what we've processed so far
		} else if err != nil {
			return written, err
		}

		// Once we start seeing these lines,
		// we are probably done with the invalid statements,
		// so we can hand off the rest to the more efficient io.Copy implementation
		for _, filterEndMarker := range filterEndMarkers {

			if bytes.HasPrefix(line, []byte(filterEndMarker)) {

				// We are probably done with the invalid statements
				noMoreLinesToFilter = true
				break

			}
		}

		if !noMoreLinesToFilter {

			for _, lineToFilter := range linesToFilter {

				if bytes.HasPrefix(line, []byte(lineToFilter)) {

					line = append([]byte("-- "), line...)
					break

				}
			}
		}

		// Write this line and update our progress before returning on error
		lineWritten, err := dst.Write(line)
		written += int64(lineWritten)
		progressFn(written)
		if err != nil {
			return written, err
		}
	}

	// No more lines to filter
	// Reset src to the last actual consumed position
	_, err := src.Seek(consumed, io.SeekStart)
	if err != nil {
		return written, errors.Wrap(err, "reset src position")
	}
	return written, nil
}
