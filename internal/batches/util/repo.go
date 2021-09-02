package util

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/batches/template"

	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

// GraphQLRepoToTemplatingRepo transforms a given *graphql.Repository into a
// template.Repository.
func GraphQLRepoToTemplatingRepo(r *graphql.Repository) template.Repository {
	return template.Repository{
		Name:        r.Name,
		FileMatches: r.FileMatches,
	}
}

func SlugForPathInRepo(repoName, commit, path string) string {
	name := repoName
	if path != "" {
		// Since path can contain os.PathSeparator or other characters that
		// don't translate well between Windows and Unix systems, we hash it.
		hash := sha256.Sum256([]byte(path))
		name = name + "-" + base64.RawURLEncoding.EncodeToString(hash[:32])
	}
	return strings.ReplaceAll(name, "/", "-") + "-" + commit
}

func SlugForRepo(repoName, commit string) string {
	return strings.ReplaceAll(repoName, "/", "-") + "-" + commit
}
