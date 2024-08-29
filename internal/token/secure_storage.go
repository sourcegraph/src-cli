//go:build !darwin

package token

import "errors"

func secure_store(service, username string, token []byte) error {
	return errors.New("not implemented")
}

func secure_retrieve(service, username string) ([]byte, error) {
	return nil, errors.New("not implemented")
}
