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

	for name, want := range map[string]OnQueryOrRepositoryCollection{
		"one query only, Vasily": {
			ons["short query"],
		},
		"one repo, one query": {
			ons["repo A"], ons["short query"],
		},
		"two queries, same length": {
			ons["short query"], ons["short query"],
		},
		"two queries, different lengths": {
			ons["long query"], ons["short query"],
		},
		"two repos, no branches": {
			ons["repo A"], ons["repo B"],
		},
		"two repos, one branch": {
			ons["repo B main"], ons["repo B"],
		},
		"two repos, two branches": {
			ons["repo B main"], ons["repo B xxx"],
		},
	} {
		t.Run(name, func(t *testing.T) {
			// For each test case, we'll try the original order we're given,
			// plus a reverse order, to ensure the sort function is actually
			// doing something.
			have := make(OnQueryOrRepositoryCollection, len(want))
			copy(have, want)

			sort.Sort(have)
			if diff := cmp.Diff(have, want); diff != "" {
				t.Error(diff)
			}

			// Reverse the slice.
			for i, j := len(want)-1, 0; i >= 0; i-- {
				have[j] = want[i]
				j++
			}

			sort.Sort(have)
			if diff := cmp.Diff(have, want); diff != "" {
				t.Error(diff)
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
