//go:build darwin

package token

import (
	"bytes"

	"github.com/keybase/go-keychain"
)

type Token struct {
	Service string
	Account string
}

func (t *Token) Save(token []byte) error {

	if existing_token, err := t.Retrieve(); err == nil {
		if bytes.Equal(token, existing_token) {
			// the stored token and supplied token are the same
			// nothing to do
			return nil
		}
		// found a token, but it is different from the supplied token, so update the token
		return keychain.UpdateItem(buildItem(t.Service, t.Account, nil), buildItem(t.Service, t.Account, token))
	} else if err != keychain.ErrorItemNotFound {
		// encountered an error checking for the token; bail now
		return err
	}

	item := buildItem(t.Service, t.Account, token)
	if err := keychain.AddItem(item); err != nil {
		if err == keychain.ErrorDuplicateItem {
			// silently skip duplicates
			// really shouldn't happen after the duplication detection above,
			// but just in case (this process is not atomic - another running `src` process could have snuck a token in the keychain)
			return nil
		}
		return err
	}
	return nil
}

func (t *Token) Retrieve() ([]byte, error) {
	query := buildItem(t.Service, t.Account, nil)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, err
	} else if len(results) != 1 {
		return nil, keychain.ErrorItemNotFound
	} else {
		return results[0].Data, nil
	}
}

func buildItem(service, username string, token []byte) keychain.Item {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlocked)
	item.SetAccessGroup("com.sourcegraph")
	item.SetService(service)
	item.SetAccount(username)
	item.SetData(token)
	item.SetComment("src-cli personal access token")
	return item
}
