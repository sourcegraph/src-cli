package campaigns

import "strings"

type OnQueryOrRepository struct {
	RepositoriesMatchingQuery string             `json:"repositoriesMatchingQuery,omitempty" yaml:"repositoriesMatchingQuery"`
	Repository                string             `json:"repository,omitempty" yaml:"repository"`
	Branch                    string             `json:"branch,omitempty" yaml:"branch"`
	ChangesetTemplate         *ChangesetTemplate `json:"changesetTemplate,omitempty" yaml:"changesetTemplate"`
}

func (on *OnQueryOrRepository) String() string {
	if on.isRepository() {
		return on.Repository
	} else {
		return on.RepositoriesMatchingQuery
	}
}

func (on *OnQueryOrRepository) isRepository() bool {
	return on.Repository != ""
}

type OnQueryOrRepositoryCollection []OnQueryOrRepository

func (coll OnQueryOrRepositoryCollection) Len() int {
	return len(coll)
}

func (coll OnQueryOrRepositoryCollection) Less(i, j int) bool {
	// The simple cases: if one or both of the OnQueryOrRepository instances
	// are repositories, then those always "win" over queries.
	if iIsRepo, jIsRepo := coll[i].isRepository(), coll[j].isRepository(); iIsRepo && jIsRepo {
		// If the repositories are the same, then we'll prioritise the
		// repository with a branch. If they both have branches, then we'll
		// punt and let the first one win, provided the collection is sorted
		// with sort.Stable().
		if coll[i].Repository == coll[j].Repository {
			if iHasBranch, jHasBranch := coll[i].Branch != "", coll[j].Branch != ""; iHasBranch && jHasBranch {
				return false
			} else if iHasBranch {
				return true
			} else if jHasBranch {
				return false
			}
		}

		// The fallback here is to just do a lexicographical comparison on the
		// repo names. They should never conflict in practice anyway, since the
		// same repo shouldn't ever be returned for disjoint repo names.
		return strings.Compare(coll[i].Repository, coll[j].Repository) < 0
	} else if iIsRepo {
		return true
	} else if jIsRepo {
		return false
	}

	// We'll apply the Traefik rule here: the longer query "wins".
	return len(coll[i].RepositoriesMatchingQuery) > len(coll[j].RepositoriesMatchingQuery)
}

func (coll OnQueryOrRepositoryCollection) Swap(i, j int) {
	coll[i], coll[j] = coll[j], coll[i]
}
