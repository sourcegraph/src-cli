// Package mock provides testify mocks for internal/api/connect.
package mock

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/sourcegraph/src-cli/internal/api/connect"
)

type Client struct {
	mock.Mock
}

func (m *Client) NewCall(procedure string, request any) connect.Call {
	args := m.Called(procedure, request)
	return args.Get(0).(connect.Call)
}

type Call struct {
	mock.Mock
}

func (c *Call) Do(ctx context.Context, response any) (bool, error) {
	args := c.Called(ctx, response)
	return args.Bool(0), args.Error(1)
}
