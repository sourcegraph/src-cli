package secrets

import (
	"github.com/99designs/keyring"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const serviceName = "sourcegraph-cli"

// keyringStore provides secure credential storage operations.
type keyringStore struct {
	ring keyring.Keyring
}

// open opens the system keyring for the Sourcegraph CLI.
func openKeyring() (*keyringStore, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName:              serviceName,
		KeychainName:             "login", // This is the default name for the keychain where MacOS puts all login passwords
		KeychainTrustApplication: true,    // the keychain can trust src-cli!
	})
	if err != nil {
		return nil, errors.Wrap(err, "opening keyring")
	}
	return &keyringStore{ring: ring}, nil
}

// Set stores a key-value pair in the keyring.
func (k *keyringStore) Put(key string, data []byte) error {
	err := k.ring.Set(keyring.Item{
		Key:   key,
		Data:  data,
		Label: key,
	})
	if err != nil {
		return errors.Wrap(err, "storing item in keyring")
	}
	return nil
}

// Get retrieves a value by key from the keyring.
// Returns nil, nil if the key is not found.
func (k *keyringStore) Get(key string) ([]byte, error) {
	item, err := k.ring.Get(key)
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return nil, ErrSecretNotFound
		}
		return nil, errors.Wrap(err, "getting item from keyring")
	}
	return item.Data, nil
}

// Delete removes a key from the keyring.
func (k *keyringStore) Delete(key string) error {
	err := k.ring.Remove(key)
	if err != nil && err != keyring.ErrKeyNotFound {
		return errors.Wrap(err, "removing item from keyring")
	}
	return nil
}
