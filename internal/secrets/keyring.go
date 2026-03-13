package secrets

import (
	"context"
	"net/url"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/zalando/go-keyring"
)

var ErrSecretNotFound = errors.New("secret not found")

const serviceNamePrefix = "Sourcegraph CLI"

type keyringStore struct {
	ctx         context.Context
	serviceName string
}

// Open opens the system keyring for the Sourcegraph CLI.
func Open(ctx context.Context, endpointURL *url.URL) (*keyringStore, error) {
	endpoint := endpointURL.String()
	if endpoint == "" {
		return nil, errors.New("endpoint cannot be empty")
	}

	serviceName := serviceNamePrefix + " <" + endpoint + ">"

	return &keyringStore{ctx: ctx, serviceName: serviceName}, nil
}

// withContext runs fn in a goroutine and returns its result, or ctx.Err() if the context is cancelled first.
func withContext[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		val T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		val, err := fn()
		ch <- result{val, err}
	}()

	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case r := <-ch:
		return r.val, r.err
	}
}

// Put stores a key-value pair in the keyring.
func (k *keyringStore) Put(key string, data []byte) error {
	_, err := withContext(k.ctx, func() (struct{}, error) {
		err := keyring.Set(k.serviceName, key, string(data))
		if err != nil {
			return struct{}{}, errors.Wrap(err, "storing item in keyring")
		}
		return struct{}{}, nil
	})
	return err
}

// Get retrieves a value by key from the keyring.
func (k *keyringStore) Get(key string) ([]byte, error) {
	return withContext(k.ctx, func() ([]byte, error) {
		secret, err := keyring.Get(k.serviceName, key)
		if err != nil {
			if err == keyring.ErrNotFound {
				return nil, ErrSecretNotFound
			}
			return nil, errors.Wrap(err, "getting item from keyring")
		}
		return []byte(secret), nil
	})
}
