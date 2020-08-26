package campaigns

import "github.com/sourcegraph/src-cli/internal/campaigns/graphql"

type Repository struct {
	*graphql.Repository

	template *ChangesetTemplate
}
