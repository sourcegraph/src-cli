package retrier

import (
	"strconv"
	"testing"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/stretchr/testify/assert"
)

func TestRetry(t *testing.T) {
	knownErr := errors.New("error!")

	t.Run("no backoff", func(t *testing.T) {
		for i := 0; i < 100; i += 10 {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Run("immediate success", func(t *testing.T) {
					retries := -1
					err := Retry(func() error {
						retries += 1
						return nil
					}, i, 0, 0)

					assert.Nil(t, err)
					assert.Equal(t, 0, retries)
				})

				t.Run("delayed success", func(t *testing.T) {
					retries := -1
					want := i / 2
					err := Retry(func() error {
						retries += 1
						if retries >= want {
							return nil
						}
						return knownErr
					}, i, 0, 0)

					assert.Nil(t, err)
					assert.Equal(t, want, retries)
				})

				t.Run("no success", func(t *testing.T) {
					retries := -1
					err := Retry(func() error {
						retries += 1
						return knownErr
					}, i, 0, 0)

					assert.NotNil(t, err)
					assert.ErrorIs(t, err, knownErr)
					assert.Equal(t, i, retries)
				})
			})
		}
	})

	t.Run("backoff", func(t *testing.T) {
		base := 1 * time.Millisecond
		want := []time.Duration{
			0, // We don't test anything on the first try.
			base,
			2 * base,
			4 * base,
			8 * base,
			16 * base,
		}

		retries := -1
		var last time.Time
		err := Retry(func() error {
			retries += 1
			if retries > 0 {
				now := time.Now()
				assert.GreaterOrEqual(t, now.Sub(last), want[retries])
				last = now
			} else {
				last = time.Now()
			}

			return knownErr
		}, 5, base, 2)

		assert.NotNil(t, err)
		assert.ErrorIs(t, err, knownErr)
		assert.Equal(t, 5, retries)
	})
}
