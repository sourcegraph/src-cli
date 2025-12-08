package secrets

import (
	"encoding/json"
	"sync"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const keyRegistry = "secret-registry"

var ErrSecretNotFound = errors.New("secret not found")

var openOnce = sync.OnceValues(Open)

type SecretStorage interface {
	Get(key string) ([]byte, error)
	Put(key string, data []byte) error
	Delete(key string) error
}

type store struct {
	backend  SecretStorage
	registry map[string][]byte

	mu sync.Mutex
}

func Store() (SecretStorage, error) {
	return openOnce()
}

func Open() (SecretStorage, error) {
	keyring, err := openKeyring()
	if err != nil {
		return nil, err
	}

	registry, err := getRegistry(keyring)
	if err != nil {
		return nil, err
	}
	s := &store{
		backend:  keyring,
		registry: registry,
	}

	return s, nil
}

func getRegistry(s SecretStorage) (map[string][]byte, error) {
	data, err := s.Get(keyRegistry)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load registry from backing store")
	}

	var registry map[string][]byte
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, errors.Wrap(err, "failed to decode registry from backing store")
	}

	return registry, nil
}

func saveRegistry(s SecretStorage, registry map[string][]byte) error {
	data, err := json.Marshal(&registry)
	if err != nil {
		return errors.Wrap(err, "registry encoding failure")
	}

	if err = s.Put(keyRegistry, data); err != nil {
		return errors.Wrap(err, "failed to persist registry to backing store")
	}

	return nil
}

func (s *store) Get(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.registry[key]
	if !ok {
		return nil, ErrSecretNotFound
	}

	return v, nil
}

func (s *store) Put(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.registry[key] = data

	return saveRegistry(s.backend, s.registry)
}

func (s *store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.registry, key)
	return saveRegistry(s.backend, s.registry)
}
