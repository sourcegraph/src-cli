package batches

import (
	"context"
	"runtime"
	"testing"

	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/batches/docker"
	"github.com/sourcegraph/src-cli/internal/batches/mock"
	"github.com/sourcegraph/src-cli/internal/exec/expect"
)

func TestBestWorkspaceCreator(t *testing.T) {
	ctx := context.Background()
	isOverridden := !(runtime.GOOS == "darwin" && runtime.GOARCH == "amd64")

	uidGid := func(uid, gid int) docker.UIDGID {
		return docker.UIDGID{UID: uid, GID: gid}
	}

	type imageBehaviour struct {
		image     string
		behaviour expect.Behaviour
	}
	for name, tc := range map[string]struct {
		images []docker.Image
		want   WorkspaceCreatorType
	}{
		"nil steps": {
			images: nil,
			want:   WorkspaceCreatorVolume,
		},
		"no steps": {
			images: []docker.Image{},
			want:   WorkspaceCreatorVolume,
		},
		"root": {
			images: []docker.Image{
				&mock.Image{UidGid: uidGid(0, 0)},
			},
			want: WorkspaceCreatorVolume,
		},
		"same user": {
			images: []docker.Image{
				&mock.Image{UidGid: uidGid(1000, 1000)},
				&mock.Image{UidGid: uidGid(1000, 1000)},
			},
			want: WorkspaceCreatorVolume,
		},
		"different user": {
			images: []docker.Image{
				&mock.Image{UidGid: uidGid(1000, 1000)},
				&mock.Image{UidGid: uidGid(0, 0)},
			},
			want: WorkspaceCreatorBind,
		},
		"id error": {
			images: []docker.Image{
				&mock.Image{UidGidErr: errors.New("foo")},
			},
			want: WorkspaceCreatorBind,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var steps []Step
			if tc.images != nil {
				steps = make([]Step, len(tc.images))
				for i, image := range tc.images {
					steps[i].image = image
				}
			}

			if isOverridden {
				// This is an overridden platform, so the workspace type will
				// always be bind from bestWorkspaceCreator().
				if have, want := BestWorkspaceCreator(ctx, steps), WorkspaceCreatorBind; have != want {
					t.Errorf("unexpected creator type on overridden platform: have=%d want=%d", have, want)
				}
			} else {
				if have := BestWorkspaceCreator(ctx, steps); have != tc.want {
					t.Errorf("unexpected creator type on non-overridden platform: have=%d want=%d", have, tc.want)
				}
			}

			// Regardless of what bestWorkspaceCreator() would have done, let's
			// test that the right thing happens regardless if detection were to
			// actually occur.
			have := detectBestWorkspaceCreator(ctx, steps)
			if have != tc.want {
				t.Errorf("unexpected creator type: have=%d want=%d", have, tc.want)
			}
		})
	}
}
