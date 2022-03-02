package retrier

import (
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// Retry retries the given function up to max times if it returns an error.
// base and multiplier may be specified to use an exponential backoff between
// retries; if no backoff is required, set both to 0.
//
// The first time the function returns nil, nil will immediately be returned
// from Retry.
func Retry(f func() error, max int, base time.Duration, multiplier float64) error {
	var errs errors.MultiError
	wait := base

	for {
		err := f()
		if err == nil {
			return nil
		}

		errs = errors.Append(errs, err)
		if len(errs.Errors()) > max {
			return errs
		}
		time.Sleep(wait)
		wait = time.Duration(multiplier * float64(wait))
	}
}
