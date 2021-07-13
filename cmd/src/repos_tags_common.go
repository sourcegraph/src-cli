package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sync"
	"text/template"

	"github.com/sourcegraph/src-cli/internal/api"
)

func makeReposTagsUsage(flagSet *flag.FlagSet, usage string) func() {
	return func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src repos tags %s:'\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
}

type repoIDCache struct {
	cache map[string]string
	mu    sync.RWMutex
}

var repoCache = &repoIDCache{cache: map[string]string{}}

func (c *repoIDCache) Get(ctx context.Context, client api.Client, name string) (string, error) {
	if id, ok := c.getCached(name); ok {
		return id, nil
	}

	var result struct {
		Repository *struct {
			ID string
		}
	}
	if ok, err := client.NewRequest(reposTagsRepoQuery, map[string]interface{}{
		"name": name,
	}).Do(ctx, &result); err != nil || !ok {
		return "", err
	}

	if result.Repository == nil {
		return "", errors.New("repository not found")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[name] = result.Repository.ID
	return result.Repository.ID, nil
}

func (c *repoIDCache) getCached(name string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	id, ok := c.cache[name]
	return id, ok
}

func showRepoTags(ctx context.Context, client api.Client, repo string, first int, highlight []string) error {
	var result struct {
		Repository *struct {
			MetadataTags struct {
				Nodes []struct {
					Tag string
				}
			}
		}
	}
	if ok, err := client.NewRequest(reposTagsQuery, map[string]interface{}{
		"first": first,
		"repo":  repo,
	}).Do(ctx, &result); err != nil || !ok {
		return err
	}

	rtctx := repoTagsContext{
		RepoName:  repo,
		Tags:      []string{},
		NotFound:  false,
		Highlight: highlight,
	}
	if result.Repository != nil {
		for _, node := range result.Repository.MetadataTags.Nodes {
			rtctx.Tags = append(rtctx.Tags, node.Tag)
		}
	} else {
		rtctx.NotFound = true
	}

	return rtctx.Render()
}

type repoTagsContext struct {
	RepoName  string
	Tags      []string
	Highlight []string
	NotFound  bool

	templateCompiler sync.Once
	template         *template.Template
	err              error
}

func (rtctx *repoTagsContext) Render() error {
	rtctx.templateCompiler.Do(func() {
		rtctx.template, rtctx.err = parseTemplate(reposTagsOutputTemplate, map[string]interface{}{
			"isHighlighted": func(tag string) bool {
				for _, highlight := range rtctx.Highlight {
					if highlight == tag {
						return true
					}
				}
				return false
			},
		})
	})
	if rtctx.err != nil {
		return rtctx.err
	}

	return execTemplate(rtctx.template, rtctx)
}

const reposTagsOutputTemplate = `
{{- color "logo" -}}✱{{- color "search-repository" }} {{ .RepoName }}{{- color "nc" -}}{{- "\n" -}}
{{- if .NotFound -}}
	{{- "\t" -}}{{- color "search-alert-title" -}}Repository does not exist{{- color "nc" -}}{{- "\n" -}}
{{- else -}}
	{{- range .Tags -}}
		{{- "\t" -}}
		{{- if isHighlighted . -}}
			{{- color "search-match" -}}
		{{- end -}}
		{{- . -}}{{- color "nc" -}}{{- "\n" -}}
	{{- else -}}
		{{- "\t" -}}{{- color "search-alert-title" -}}No results{{- color "nc" -}}{{- "\n" -}}
	{{- end -}}
{{- end -}}
`

const reposTagsQuery = `
	query RepoTags($first: Int!, $repo: String!) {
		repository(name: $repo) {
			metadataTags(first: $first) {
				nodes {
					tag
				}
			}
		}
	}
`

const reposTagsRepoQuery = `
	query RepositoryByName($name: String!) {
		repository(name: $name) {
			id
		}
	}
`
