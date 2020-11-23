package graphql

import (
	"sort"
	"strings"
)

const RepositoryFieldsFragment = `
fragment repositoryFields on Repository {
    id
    name
    url
    externalRepository {
        serviceType
    }
    defaultBranch {
        name
        target {
            oid
        }
    }
}
`

const RepositoryWithBranchFragment = `
fragment repositoryFieldsWithBranch on Repository {
    id
    name
    url
    externalRepository {
        serviceType
    }
    defaultBranch {
        name
        target {
            oid
        }
    }
	branches(query: $branch, first: 1) @include(if:$queryBranch){
	    nodes {
		    name
            target {
                oid
            }
		}
	}
}
`

type Target struct {
	OID string
}

type Branch struct {
	Name   string
	Target Target
}

type Branches struct {
	Nodes []*Branch
}

type Repository struct {
	ID                 string
	Name               string
	URL                string
	ExternalRepository struct{ ServiceType string }

	DefaultBranch *Branch
	Branches      Branches

	FileMatches map[string]bool
}

func (r *Repository) HasBranch() bool {
	return r.DefaultBranch != nil || len(r.Branches.Nodes) != 0
}

func (r *Repository) BaseRef() string {
	if len(r.Branches.Nodes) != 0 {
		return r.Branches.Nodes[0].Name
	}

	return r.DefaultBranch.Name
}

func (r *Repository) Rev() string {
	if len(r.Branches.Nodes) != 0 {
		return r.Branches.Nodes[0].Target.OID
	}

	return r.DefaultBranch.Target.OID
}

func (r *Repository) Slug() string {
	return strings.ReplaceAll(r.Name, "/", "-")
}

func (r *Repository) SearchResultPaths() (list fileMatchPathList) {
	var files []string
	for f := range r.FileMatches {
		files = append(files, f)
	}
	sort.Strings(files)
	return fileMatchPathList(files)
}

type fileMatchPathList []string

func (f fileMatchPathList) String() string { return strings.Join(f, " ") }
