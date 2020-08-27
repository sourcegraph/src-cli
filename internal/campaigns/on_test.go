package campaigns

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestOn_SortInterface(t *testing.T) {
	// Set up some standard OnQueryOrRepository variables to use in test cases.
	ons := map[string]OnQueryOrRepository{
		"repo A":      {Repository: "github.com/a/a"},
		"repo B":      {Repository: "github.com/a/b"},
		"repo B main": {Repository: "github.com/a/b", Branch: "main"},
		"repo B xxx":  {Repository: "github.com/a/b", Branch: "xxx"},
		"short query": {RepositoriesMatchingQuery: "f:README.md"},
		"long query":  {RepositoriesMatchingQuery: "f:README.md r:a/b"},
	}

	for name, tc := range map[string]struct {
		want    OnQueryOrRepositoryCollection
		reverse bool
	}{
		"one query only, Vasily": {
			want: OnQueryOrRepositoryCollection{
				ons["short query"],
			},
			reverse: true,
		},
		"one repo, one query": {
			want: OnQueryOrRepositoryCollection{
				ons["repo A"], ons["short query"],
			},
			reverse: true,
		},
		"two queries, same length": {
			want: OnQueryOrRepositoryCollection{
				ons["short query"], ons["short query"],
			},
			reverse: true,
		},
		"two queries, different lengths": {
			want: OnQueryOrRepositoryCollection{
				ons["long query"], ons["short query"],
			},
			reverse: true,
		},
		"two repos, no branches": {
			want: OnQueryOrRepositoryCollection{
				ons["repo A"], ons["repo B"],
			},
			reverse: true,
		},
		"two repos, one branch": {
			want: OnQueryOrRepositoryCollection{
				ons["repo B main"], ons["repo B"],
			},
			reverse: true,
		},
		"two repos, two branches": {
			want: OnQueryOrRepositoryCollection{
				ons["repo B main"], ons["repo B xxx"],
			},
			// We don't want to do a reverse test here because it relies on
			// stable sorting, since the collection items are considered equal.
			reverse: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			have := make(OnQueryOrRepositoryCollection, len(tc.want))
			copy(have, tc.want)

			sort.Stable(have)
			if diff := cmp.Diff(have, tc.want); diff != "" {
				t.Error(diff)
			}

			if tc.reverse {
				for i, j := len(tc.want)-1, 0; i >= 0; i-- {
					have[j] = tc.want[i]
					j++
				}

				sort.Stable(have)
				if diff := cmp.Diff(have, tc.want); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}

func TestOn_String(t *testing.T) {
	for want, on := range map[string]OnQueryOrRepository{
		"":               {},
		"github.com/a/b": {Repository: "github.com/a/b", Branch: "main"},
		"f:README.md":    {RepositoriesMatchingQuery: "f:README.md"},
		"github.com/a/c": {
			Repository:                "github.com/a/c",
			RepositoriesMatchingQuery: "this is invalid, but let's test it anyway",
		},
	} {
		t.Run(want, func(t *testing.T) {
			if have := on.String(); have != want {
				t.Errorf("have=%q want=%q", have, want)
			}
		})
	}
}
