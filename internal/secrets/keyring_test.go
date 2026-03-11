package secrets

import (
	"context"
	"net/url"
	"testing"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		name            string
		endpoint        *url.URL
		wantServiceName string
		wantErr         bool
	}{
		{
			name:            "simple endpoint",
			endpoint:        &url.URL{Scheme: "https", Host: "sourcegraph.example.com"},
			wantServiceName: "Sourcegraph CLI <https://sourcegraph.example.com>",
		},
		{
			name:            "endpoint with path",
			endpoint:        &url.URL{Scheme: "https", Host: "sourcegraph.example.com", Path: "/sourcegraph"},
			wantServiceName: "Sourcegraph CLI <https://sourcegraph.example.com/sourcegraph>",
		},
		{
			name:            "endpoint with nested path",
			endpoint:        &url.URL{Scheme: "https", Host: "sourcegraph.example.com", Path: "/custom/path"},
			wantServiceName: "Sourcegraph CLI <https://sourcegraph.example.com/custom/path>",
		},
		{
			name:     "empty endpoint",
			endpoint: &url.URL{Scheme: "", Host: ""},
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
