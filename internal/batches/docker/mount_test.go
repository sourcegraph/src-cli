package docker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBindMount(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		target   string
		readOnly bool
		want     string
		wantErr  bool
	}{
		{
			name:   "writable",
			source: "/tmp/workspace",
			target: "/work",
			want:   "type=bind,source=/tmp/workspace,target=/work",
		},
		{
			name:     "read-only",
			source:   "/tmp/archive",
			target:   "/tmp/archive",
			readOnly: true,
			want:     "type=bind,source=/tmp/archive,target=/tmp/archive,ro",
		},
		{
			name:    "source injection",
			source:  "/tmp/archive,source=/etc",
			target:  "/tmp/archive",
			wantErr: true,
		},
		{
			name:    "target injection",
			source:  "/tmp/archive",
			target:  "/tmp/archive,source=/etc",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := BindMount(test.source, test.target, test.readOnly)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
