package graphql

import "strings"

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

type Branch struct {
	Name   string
	Target struct{ OID string }
}

type Repository struct {
	ID                 string
	Name               string
	URL                string
	ExternalRepository struct{ ServiceType string }
	DefaultBranch      *Branch

	FileMatches map[string]bool
}

func (r *Repository) BaseRef() string {
	return r.DefaultBranch.Name
}

func (r *Repository) Rev() string {
	return r.DefaultBranch.Target.OID
}

func (r *Repository) Slug() string {
	return strings.ReplaceAll(r.Name, "/", "-")
}

func (r *Repository) FileMatchPaths() string {
	files := []string{}
	for f := range r.FileMatches {
		files = append(files, f)
	}
	return strings.Join(files, " ")
}

type fileMatchPathList []string

func (f fileMatchPathList) String() string { return strings.Join(f, " ") }

func (r *Repository) FileMatchPathList() (list fileMatchPathList) {
	for f := range r.FileMatches {
		list = append(list, f)
	}
	return list
}
