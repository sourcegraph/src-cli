// Package keyring provides secure credential storage using the system keychain.
package keyring

import (
	"github.com/99designs/keyring"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const serviceName = "sourcegraph-cli"

// Store provides secure credential storage operations.
type Store struct {
	ring keyring.Keyring
}

// Open opens the system keyring for the Sourcegraph CLI.
func Open() (*Store, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName:              serviceName,
		KeychainName:             "login", // This is the default name for the keychain where MacOS puts all login passwords
		KeychainTrustApplication: true,    // the keychain can trust src-cli!
	})
	if err != nil {
		return nil, errors.Wrap(err, "opening keyring")
	}
	return &Store{ring: ring}, nil
}

// Set stores a key-value pair in the keyring.
func (s *Store) Set(key string, data []byte) error {
	err := s.ring.Set(keyring.Item{
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
func (s *Store) Get(key string) ([]byte, error) {
	item, err := s.ring.Get(key)
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return nil, nil
		}
		return nil, errors.Wrap(err, "getting item from keyring")
	}
	return item.Data, nil
}

// Delete removes a key from the keyring.
func (s *Store) Delete(key string) error {
	err := s.ring.Remove(key)
	if err != nil && err != keyring.ErrKeyNotFound {
		return errors.Wrap(err, "removing item from keyring")
	}
	return nil
}
