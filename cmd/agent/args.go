package main

import (
	"context"
	"errors"
	"os"
	"reflect"

	"github.com/sourcegraph/src-cli/internal/api"
)

var errNoCommand = errors.New("no command given")

type Args struct {
	Register *RegisterCmd `arg:"subcommand:register"`

	Endpoint string `arg:"env:SRC_ENDPOINT"`
}

func (a *Args) client() api.Client {
	return api.NewClient(api.ClientOpts{
		Endpoint: a.Endpoint,
		Out:      os.Stderr,
	})
}

func (a *Args) authenticatedClient(token string) api.Client {
	return api.NewClient(api.ClientOpts{
		Endpoint:    a.Endpoint,
		AccessToken: token,
		Out:         os.Stderr,
	})
}

func (a *Args) dispatch(ctx context.Context) error {
	// Dispatch to any possible command in the arguments.
	v := reflect.ValueOf(args)
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.Ptr && !f.IsNil() {
			if cmd, ok := v.Field(i).Interface().(command); ok {
				if err := cmd.Execute(ctx, &args); err != nil {
					return err
				} else {
					return nil
				}
			}
		}
	}

	return errNoCommand
}
