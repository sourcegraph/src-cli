package pgdump

import (
	"bufio"
	"bytes"
	"io"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// FilterInvalidLines copies the initial lines of the pg_dump-created .sql files,
// from src to dst (the GCS bucket),
// until it hits a line prefixed with a filterEndMarker,
// while commenting out the linesToFilter which cause `gcloud sql import` to error out.
// It then resets src to the position of the last contents written to dst.
//
// Filtering requires reading entire lines into memory,
// this can be a very expensive operation, so when filtering is complete,
// the more efficient io.Copy is used to perform the remainder of the copy in the calling funciton
//
// pg_dump writes these .sql files based on its own version,
// not based on the Postgres version of either the source or destination database;
// so self-hosted customers' diverse database environments
// have inserted a variety of statements into the .sql files which cause the import to fail
// For details, see https://cloud.google.com/sql/docs/postgres/import-export/import-export-dmp
func FilterInvalidLines(dst io.Writer, src io.ReadSeeker, progressFn func(int64)) (int64, error) {
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

			// Cloud instances' databases have been upgraded to Postgres v16.10,
			// which should include support for \restrict and \unrestrict
			// but leaving in the list in case we need to re-add them
			// "\\restrict",
			// To handle the \unrestrict command,
			// we'd have to add a search from the end of the file
			// "\\unrestrict",
			// Remove comments after databases are upgraded >= Postgres 17
		}
	)

	for !noMoreLinesToFilter {

		// Read up to a line, keeping track of our position in src
		line, err := reader.ReadBytes('\n')
		consumed += int64(len(line))

		// If this function has read through the whole file without hitting a filterEndMarker,
		// then handle the last line correctly
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
