package secrets

import (
	"context"
	"testing"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		name            string
		endpoint        string
		wantServiceName string
		wantErr         bool
	}{
		{
			name:            "normalized endpoint",
			endpoint:        " https://sourcegraph.example.com/ ",
			wantServiceName: "Sourcegraph CLI <https://sourcegraph.example.com>",
		},
		{
			name:            "normalized endpoint with path",
			endpoint:        " https://sourcegraph.example.com/sourcegraph/ ",
			wantServiceName: "Sourcegraph CLI <https://sourcegraph.example.com/sourcegraph>",
		},
		{
			name:            "normalized endpoint with nested path",
			endpoint:        "https://sourcegraph.example.com/custom/path///",
			wantServiceName: "Sourcegraph CLI <https://sourcegraph.example.com/custom/path>",
		},
		{
			name:     "empty endpoint",
			endpoint: " / ",
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := Open(context.Background(), test.endpoint)
			if test.wantErr {
				if err == nil {
					t.Fatal("Open() error = nil, want non-nil")
				}
				if store != nil {
					t.Fatalf("Open() store = %v, want nil", store)
				}
				return
			}

			if err != nil {
				t.Fatalf("Open() error = %v, want nil", err)
			}
			if got := store.serviceName; got != test.wantServiceName {
				t.Fatalf("Open() serviceName = %q, want %q", got, test.wantServiceName)
			}
		})
	}
}
