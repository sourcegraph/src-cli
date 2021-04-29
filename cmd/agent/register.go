package main

import (
	"context"
	"fmt"
	"os"
)

type RegisterCmd struct {
	Name  string `arg:"-n,--name" help:"name to identify this worker with; hostname by default"`
	Token string `arg:"env:SRC_ACCESS_TOKEN" help:"the API token to use when registering"`
}

var _ command = &RegisterCmd{}

func (rc *RegisterCmd) Execute(ctx context.Context, args *Args) error {
	client := args.authenticatedClient(rc.Token)

	if rc.Name == "" {
		var err error
		if rc.Name, err = os.Hostname(); err != nil {
			return err
		}
	}

	var result struct {
		RegisterBatchWorker struct {
			Token string
		}
	}

	if ok, err := client.NewRequest(registerMutation, map[string]interface{}{
		"name": rc.Name,
	}).Do(ctx, result); err != nil || !ok {
		return err
	}

	fmt.Printf("token: %s\n", result.RegisterBatchWorker.Token)
	return nil
}

const registerMutation = `
mutation RegisterWorker($name: String!) {
	registerBatchWorker(name: $name) {
		token
	}
}
`
