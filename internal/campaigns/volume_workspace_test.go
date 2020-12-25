//+build !windows

package campaigns

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/alessio/shellescape"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

func TestVolumeWorkspace_Close(t *testing.T) {
	ctx := context.Background()

	for name, tc := range map[string]struct {
		w       *dockerVolumeWorkspace
		wantErr bool
	}{
		"success": {
			w:       &dockerVolumeWorkspace{volume: "VOLUME"},
			wantErr: false,
		},
		"failure": {
			w:       &dockerVolumeWorkspace{volume: "FOO"},
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var (
				success = 0
				failure = 1
			)
			if tc.wantErr {
				success = 1
				failure = 0
			}

			ec, err := expectCommand(success, failure, "docker", "volume", "rm", tc.w.volume)
			if err != nil {
				t.Fatal(err)
			}
			defer ec.finalise(t)

			if err = tc.w.Close(ctx); tc.wantErr && err == nil {
				t.Error("unexpected nil error")
			} else if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestVolumeWorkspace_DockerRunOpts(t *testing.T) {
	ctx := context.Background()
	w := &dockerVolumeWorkspace{volume: "VOLUME"}

	want := []string{
		"--mount", "type=volume,source=VOLUME,target=TARGET",
	}
	have, err := w.DockerRunOpts(ctx, "TARGET")
	if err != nil {
		t.Errorf("unexpected error: %+v", err)
	}
	if diff := cmp.Diff(have, want); diff != "" {
		t.Errorf("unexpected options (-have +want):\n%s", diff)
	}
}

func TestVolumeWorkspace_WorkDir(t *testing.T) {
	if have := (&dockerVolumeWorkspace{}).WorkDir(); have != nil {
		t.Errorf("unexpected work dir: %q", *have)
	}
}

func expectCommand(success, failure int, command string, args ...string) (*expectedCommand, error) {
	dir, err := ioutil.TempDir(os.TempDir(), "expect-*")
	if err != nil {
		return nil, errors.Wrap(err, "creating temporary directory to contain the expected command")
	}

	called := path.Join(dir, command+".called")
	script := `#!/bin/sh

set -e
set -x

retval=` + strconv.Itoa(success)
	for i, arg := range args {
		script += `
if [ "$1" != ` + shellescape.Quote(arg) + ` ]; then
	echo "Argument ` + strconv.Itoa(i+1) + ` is invalid: $1"
	retval=` + strconv.Itoa(failure) + `
fi
shift >/dev/null 2>&1
`
	}
	script += `
if [ "$retval" -eq ` + strconv.Itoa(success) + ` ]; then
	touch ` + shellescape.Quote(called) + `
fi

exit "$retval"`
	dest := path.Join(dir, command)
	if err := ioutil.WriteFile(dest, []byte(script), 0755); err != nil {
		return nil, errors.Wrapf(err, "writing script to %q", dest)
	}

	oldPath := os.Getenv("PATH")
	if oldPath != "" {
		os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	} else {
		os.Setenv("PATH", dir)
	}

	return &expectedCommand{
		called:  called,
		dir:     dir,
		oldPath: oldPath,
	}, nil
}

type expectedCommand struct {
	called  string
	dir     string
	oldPath string
}

func (ec *expectedCommand) finalise(t *testing.T) {
	t.Helper()

	called := true
	if _, err := os.Stat(path.Join(ec.called)); os.IsNotExist(err) {
		called = false
	} else if err != nil {
		t.Fatalf("error stating called file %q: %v", ec.called, err)
	}

	os.Setenv("PATH", ec.oldPath)
	if err := os.RemoveAll(ec.dir); err != nil {
		t.Fatalf("error removing expected command directory %q: %v", ec.dir, err)
	}

	if !called {
		t.Error("expected command not called")
	}
}
