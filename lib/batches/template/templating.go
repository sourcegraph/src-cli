package template

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/gobwas/glob"
	"github.com/grafana/regexp"

	"github.com/kballard/go-shellquote"
	"github.com/sourcegraph/sourcegraph/lib/batches/execution"
	"github.com/sourcegraph/sourcegraph/lib/batches/git"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const startDelim = "${{"
const endDelim = "}}"

// shellEscapeAll returns a copy of in where every element has been quoted
// with shellquote.Join so that it is safe to splat into a /bin/sh command line.
// Elements without shell metacharacters are returned unmodified, so callers
// that pre-existed this escaping (e.g. `${{ join steps.modified_files " " }}`
// against safe filenames) continue to see the same rendered output.
//
// 🚨 SECURITY: This is used to defang attacker-controlled filenames coming
// out of `git diff` parsing before they reach the rendered step script. See
// VULN-91 / HackerOne report 3767160.
func shellEscapeAll(in ...string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = shellquote.Join(s)
	}
	return out
}

// isCanonicallyShellQuoted reports whether s is exactly what shellquote.Join
// would produce for some single-element argv: i.e. s parses as one shell
// word whose re-quoted canonical form is identical to s.
//
// This lets `shellquote_join` be idempotent — calling it on a value that
// src-cli already pre-escaped (e.g. each element of
// `previous_step.modified_files`) or on the output of a previous
// `shellquote_join` returns the value unchanged instead of double-escaping
// it into a literal filename that the shell would then look up verbatim.
//
// The check is content-based, so it works regardless of how the value
// reached the template (direct slice access, captured outputs, raw stdio,
// hand-built argv, etc.) — types do not have to survive the trip through
// `outputs` serialization and template rendering.
func isCanonicallyShellQuoted(s string) bool {
	parts, err := shellquote.Split(s)
	if err != nil || len(parts) != 1 {
		return false
	}
	return shellquote.Join(parts[0]) == s
}

// Builtin template functions available inside any batch spec template.
//
// Quick reference for what to use when a value flows into a /bin/sh command:
//
//   - Splatting `repository.search_result_paths`, `*_files`, or any other
//     filename slice that src-cli pre-escapes for you: use plain `join`.
//     These slices already have every element wrapped with shellquote.Join
//     so they are safe to splat as-is. Reaching for `shellquote_join` here
//     is unnecessary but also harmless — see the idempotence note below.
//
//   - Splatting a string or slice that you built yourself from raw stdio
//     (`previous_step.stdout`), captured `outputs.*` values, search results
//     fed back through outputs, or any other source where the bytes are not
//     already canonical-quoted: use `shellquote_join`.
//
//   - Re-parsing a shell-formatted string back into an argv: use
//     `shellquote_split` (optionally piping back through `shellquote_join`).
//
// String helpers (all from the Go standard library):
//
//   - join     — strings.Join(elems, sep). Concatenates the elements of a
//     slice with the given separator. Performs NO shell quoting
//     itself, so it is only safe on slices whose elements are
//     either already shell-quoted (the src-cli `*_files` and
//     `repository.search_result_paths` fields) or trusted not
//     to contain shell metacharacters. For anything else,
//     reach for `shellquote_join`.
//   - split    — strings.Split(s, sep). Splits a string around every
//     occurrence of sep into a slice.
//   - replace  — strings.ReplaceAll(s, old, new). Replaces every occurrence
//     of old in s with new.
//   - join_if  — Joins elems with sep, skipping any empty strings.
//   - matches  — Reports whether the input matches the given glob pattern.
//
// Shell-quoting helpers (from github.com/kballard/go-shellquote):
//
//   - shellquote_join  — Joins its arguments into a single string with each
//     element shell-quoted. Accepts either a single
//     []string or any number of individual string args
//     (or a mix of both), so it works equally well on
//     slice variables and on a hand-built argv. Example:
//
//     run: gofmt -w ${{ shellquote_join outputs.argv }}
//     run: my-tool ${{ shellquote_join "--name" outputs.userName }}
//
//     Element-level calls are idempotent: every string
//     that is already in canonical shellquote form (each
//     element of `*_files` and
//     `repository.search_result_paths`, or anything you
//     hand-quoted yourself) is passed through unchanged
//     instead of being double-escaped. So if you do reach
//     for `shellquote_join` on a pre-escaped slice you
//     will get the same output as `join … " "`, not a
//     corrupted second layer of quoting.
//
//     Note: this idempotence is per-element. If you pass
//     a SINGLE string that itself happens to look like a
//     multi-word canonical argv (e.g. the output of a
//     previous shellquote_join captured into outputs),
//     it is treated as one shell word and re-quoted as a
//     whole. Keep the slice shape if you intend to feed
//     the result through shellquote_join a second time.
//
//   - shellquote_split — shellquote.Split(input). The inverse of
//     shellquote_join: parses a shell-quoted string back
//     into the original slice of arguments, honouring
//     quoting, escaping and backslash rules in the same
//     way /bin/sh's word-splitting does. Useful when a
//     previous step (or a user-provided output) hands you
//     a single shell-formatted string that you need to
//     iterate over or re-quote. Example:
//
//     run: |
//     for f in ${{ shellquote_join (shellquote_split outputs.fileList) }}; do
//     process "$f"
//     done
//
//     Returns an error (which surfaces as a template
//     execution failure) if the input contains an
//     unterminated quote.
var builtins = template.FuncMap{
	"join": strings.Join,
	// shellquote_join accepts either a single []string or variadic string
	// arguments and returns a single shell-quoted string.
	//
	// We expose it as `func(...any) (string, error)` rather than
	// `shellquote.Join` directly for two reasons:
	//
	//  1. Go's text/template does NOT splat a []string into a variadic
	//     ...string parameter, so a bare shellquote.Join would reject the
	//     natural `${{ shellquote_join (shellquote_split foo) }}` round-trip
	//     and `${{ shellquote_join outputs.argv }}` (where argv is a slice).
	//
	//  2. Elements that are already in canonical shellquote form are passed
	//     through unchanged so the function is idempotent. This prevents the
	//     common footgun of calling `shellquote_join` on a value that
	//     src-cli already pre-escaped (every element of `*_files` and
	//     `repository.search_result_paths`, or the result of a previous
	//     `shellquote_join`) and getting a literal-quoted filename that the
	//     shell would then look up verbatim. See isCanonicallyShellQuoted.
	"shellquote_join": func(args ...any) (string, error) {
		var flat []string
		appendOne := func(s string) {
			if isCanonicallyShellQuoted(s) {
				flat = append(flat, s)
				return
			}
			flat = append(flat, shellquote.Join(s))
		}
		for _, a := range args {
			switch v := a.(type) {
			case string:
				appendOne(v)
			case []string:
				for _, s := range v {
					appendOne(s)
				}
			case []any:
				for i, e := range v {
					s, ok := e.(string)
					if !ok {
						return "", errors.Newf("shellquote_join: element %d is %T, want string", i, e)
					}
					appendOne(s)
				}
			default:
				return "", errors.Newf("shellquote_join: unsupported argument type %T", a)
			}
		}
		// flat is already per-element canonical-quoted, so a plain
		// strings.Join(" ") would suffice. We use shellquote.Join's
		// space-separated concatenation for symmetry and to keep the public
		// contract "this returns a canonical shellquote.Join string".
		return strings.Join(flat, " "), nil
	},
	"shellquote_split": shellquote.Split,
	"split":            strings.Split,
	"replace":          strings.ReplaceAll,
	"join_if": func(sep string, elems ...string) string {
		var nonBlank []string
		for _, e := range elems {
			if e != "" {
				nonBlank = append(nonBlank, e)
			}
		}
		return strings.Join(nonBlank, sep)
	},
	"matches": func(in, pattern string) (bool, error) {
		g, err := glob.Compile(pattern)
		if err != nil {
			return false, err
		}
		return g.Match(in), nil
	},
}

// ValidateBatchSpecTemplate attempts to perform a dry run replacement of the whole batch
// spec template for any templating variables which are not dependent on execution
// context. It returns a tuple whose first element is whether or not the batch spec is
// valid and whose second element is an error message if the spec is found to be invalid.
func ValidateBatchSpecTemplate(spec string) (bool, error) {
	// We use empty contexts to create "dummy" `template.FuncMap`s -- function mappings
	// with all the right keys, but no actual values. We'll use these `FuncMap`s to do a
	// dry run on the batch spec to determine if it's valid or not, before we actually
	// execute it.
	sc := &StepContext{}
	sfm := sc.ToFuncMap()
	cstc := &ChangesetTemplateContext{}
	cstfm := cstc.ToFuncMap()

	// Strip any use of `outputs` fields from the spec template. Without using real
	// contexts for the `FuncMap`s, they'll fail to `template.Execute`, and it's difficult
	// to statically validate them without deeper inspection of the YAML, so our
	// validation is just a best-effort without them.
	outputRe := regexp.MustCompile(`(?i)\$\{\{\s*[^}]*\s*outputs\.[^}]*\}\}`)
	spec = outputRe.ReplaceAllString(spec, "")

	// Also strip index references. We also can't validate whether or not an index is in
	// range without real context.
	indexRe := regexp.MustCompile(`(?i)\$\{\{\s*index\s*[^}]*\}\}`)
	spec = indexRe.ReplaceAllString(spec, "")

	// By default, text/template will continue even if it encounters a key that is not
	// indexed in any of the provided `FuncMap`s. A missing key is an indication of an
	// unknown or mistyped template variable which would invalidate the batch spec, so we
	// want to fail immediately if we encounter one. We accomplish this by setting the
	// option "missingkey=error". See https://pkg.go.dev/text/template#Template.Option for
	// more.
	t, err := New("validateBatchSpecTemplate", spec, "missingkey=error", sfm, cstfm)

	if err != nil {
		// Attempt to extract the specific template variable field that caused the error
		// to provide a clearer message.
		errorRe := regexp.MustCompile(`(?i)function "(?P<key>[^"]+)" not defined`)
		if matches := errorRe.FindStringSubmatch(err.Error()); len(matches) > 0 {
			return false, errors.New(fmt.Sprintf("validating batch spec template: unknown templating variable: '%s'", matches[1]))
		}
		// If we couldn't give a more specific error, fall back on the one from text/template.
		return false, errors.Wrap(err, "validating batch spec template")
	}

	var out bytes.Buffer
	if err = t.Execute(&out, &StepContext{}); err != nil {
		// Attempt to extract the specific template variable fields that caused the error
		// to provide a clearer message.
		errorRe := regexp.MustCompile(`(?i)at <(?P<outer>[^>]+)>:.*for key "(?P<inner>[^"]+)"`)
		if matches := errorRe.FindStringSubmatch(err.Error()); len(matches) > 0 {
			return false, errors.New(fmt.Sprintf("validating batch spec template: unknown templating variable: '%s.%s'", matches[1], matches[2]))
		}
		// If we couldn't give a more specific error, fall back on the one from text/template.
		return false, errors.Wrap(err, "validating batch spec template")
	}

	return true, nil
}

func isTrueOutput(output interface{ String() string }) bool {
	return strings.TrimSpace(output.String()) == "true"
}

func EvalStepCondition(condition string, stepCtx *StepContext) (bool, error) {
	if condition == "" {
		return true, nil
	}

	var out bytes.Buffer
	if err := RenderStepTemplate("step-condition", condition, &out, stepCtx); err != nil {
		return false, errors.Wrap(err, "parsing step if")
	}

	return isTrueOutput(&out), nil
}

func RenderStepTemplate(name, tmpl string, out io.Writer, stepCtx *StepContext) error {
	// By default, text/template will continue even if it encounters a key that is not
	// indexed in any of the provided `FuncMap`s, replacing the variable with "<no
	// value>". This means that a mis-typed variable such as "${{
	// repository.search_resalt_paths }}" would just be evaluated as "<no value>", which
	// is not a particularly useful substitution and will only indirectly manifest to the
	// user as an error during execution. Instead, we prefer to fail immediately if we
	// encounter an unknown variable. We accomplish this by setting the option
	// "missingkey=error". See https://pkg.go.dev/text/template#Template.Option for more.
	t, err := New(name, tmpl, "missingkey=error", stepCtx.ToFuncMap())
	if err != nil {
		return errors.Wrap(err, "parsing step run")
	}

	return t.Execute(out, stepCtx)
}

func RenderStepMap(m map[string]string, stepCtx *StepContext) (map[string]string, error) {
	rendered := make(map[string]string, len(m))

	for k, v := range m {
		var out bytes.Buffer

		if err := RenderStepTemplate(k, v, &out, stepCtx); err != nil {
			return rendered, err
		}

		rendered[k] = out.String()
	}

	return rendered, nil
}

// TODO(mrnugget): This is bad and should be (a) removed or (b) moved to batches package
type BatchChangeAttributes struct {
	Name        string
	Description string
}

type Repository struct {
	Name        string
	Branch      string
	FileMatches []string
}

// SearchResultPaths returns the repository's matched paths in a form that is
// safe to splat into a /bin/sh command line.
//
// 🚨 SECURITY: paths originate from a Sourcegraph search and ultimately from
// `git`, so they may contain attacker-controlled shell metacharacters (see
// VULN-91). Every element is run through shellquote.Join before it is exposed
// to the step template. Elements without metacharacters are returned
// unmodified, so existing usage like `${{ join repository.search_result_paths
// " " }}` keeps producing the same output for benign filenames.
func (r Repository) SearchResultPaths() (list fileMatchPathList) {
	paths := slices.Clone(r.FileMatches)
	sort.Strings(paths)
	for i, p := range paths {
		paths[i] = shellquote.Join(p)
	}
	return paths
}

type fileMatchPathList []string

func (f fileMatchPathList) String() string { return strings.Join(f, " ") }

// StepContext represents the contextual information available when rendering a
// step's fields, such as "run" or "outputs", as templates.
type StepContext struct {
	// BatchChange are the attributes in the BatchSpec that are set on the BatchChange.
	BatchChange BatchChangeAttributes
	// Outputs are the outputs set by the current and all previous steps.
	Outputs map[string]any
	// Step is the result of the current step. Empty when evaluating the "run" field
	// but filled when evaluating the "outputs" field.
	Step execution.AfterStepResult
	// Steps contains the path in which the steps are being executed and the
	// changes made by all steps that were executed up until the current step.
	Steps StepsContext
	// PreviousStep is the result of the previous step. Empty when there is no
	// previous step.
	PreviousStep execution.AfterStepResult
	// Repository is the Sourcegraph repository in which the steps are executed.
	Repository Repository
}

// ToFuncMap returns a template.FuncMap to access fields on the StepContext in a
// text/template.
func (stepCtx *StepContext) ToFuncMap() template.FuncMap {
	newStepResult := func(res *execution.AfterStepResult) map[string]any {
		m := map[string]any{
			"modified_files": "",
			"added_files":    "",
			"deleted_files":  "",
			"renamed_files":  "",
			"stdout":         "",
			"stderr":         "",
		}
		if res == nil {
			return m
		}

		// 🚨 SECURITY: file lists are derived from `git diff` output and can
		// contain attacker-controlled filenames with shell metacharacters.
		// We shell-escape each element before exposing them to the step
		// template to prevent command injection when the rendered template
		// is executed by /bin/sh. The slice shape is preserved so
		// `${{ join … " " }}` and `${{ range … }}` continue to work as
		// before. See VULN-91.
		//
		// NOTE: stdout/stderr are intentionally NOT pre-escaped. They are
		// commonly captured into `outputs` and reused as plain values (e.g.
		// a filename written by `echo`), where pre-quoting would change
		// semantics. Users that splat stdout/stderr into a shell command
		// against untrusted data should pipe through the `shellquote_join`
		// builtin: `${{ shellquote_join previous_step.stdout }}`.
		m["modified_files"] = shellEscapeAll(res.ChangedFiles.Modified...)
		m["added_files"] = shellEscapeAll(res.ChangedFiles.Added...)
		m["deleted_files"] = shellEscapeAll(res.ChangedFiles.Deleted...)
		m["renamed_files"] = shellEscapeAll(res.ChangedFiles.Renamed...)
		m["stdout"] = res.Stdout
		m["stderr"] = res.Stderr

		return m
	}

	return template.FuncMap{
		"previous_step": func() map[string]any {
			return newStepResult(&stepCtx.PreviousStep)
		},
		"step": func() map[string]any {
			return newStepResult(&stepCtx.Step)
		},
		"steps": func() map[string]any {
			res := newStepResult(&execution.AfterStepResult{ChangedFiles: stepCtx.Steps.Changes})
			res["path"] = stepCtx.Steps.Path
			return res
		},
		"outputs": func() map[string]any {
			return stepCtx.Outputs
		},
		"repository": func() map[string]any {
			return map[string]any{
				"search_result_paths": stepCtx.Repository.SearchResultPaths(),
				"name":                stepCtx.Repository.Name,
				"branch":              stepCtx.Repository.Branch,
			}
		},
		"batch_change": func() map[string]any {
			return map[string]any{
				"name":        stepCtx.BatchChange.Name,
				"description": stepCtx.BatchChange.Description,
			}
		},
	}
}

type StepsContext struct {
	// Changes that have been made by executing all steps.
	Changes git.Changes
	// Path is the relative-to-root directory in which the steps have been
	// executed. Default is "". No leading "/".
	Path string
}

// ChangesetTemplateContext represents the contextual information available
// when rendering a field of the ChangesetTemplate as a template.
type ChangesetTemplateContext struct {
	// BatchChangeAttributes are the attributes of the BatchChange that will be
	// created/updated.
	BatchChangeAttributes BatchChangeAttributes

	// Steps are the changes made by all steps that were executed.
	Steps StepsContext

	// Outputs are the outputs defined and initialized by the steps.
	Outputs map[string]any

	// Repository is the repository in which the steps were executed.
	Repository Repository
}

// ToFuncMap returns a template.FuncMap to access fields on the StepContext in a
// text/template.
func (tmplCtx *ChangesetTemplateContext) ToFuncMap() template.FuncMap {
	return template.FuncMap{
		"repository": func() map[string]any {
			return map[string]any{
				"search_result_paths": tmplCtx.Repository.SearchResultPaths(),
				"name":                tmplCtx.Repository.Name,
				"branch":              tmplCtx.Repository.Branch,
			}
		},
		"batch_change": func() map[string]any {
			return map[string]any{
				"name":        tmplCtx.BatchChangeAttributes.Name,
				"description": tmplCtx.BatchChangeAttributes.Description,
			}
		},
		"outputs": func() map[string]any {
			return tmplCtx.Outputs
		},
		"steps": func() map[string]any {
			// 🚨 SECURITY: shell-escape per element to defang attacker
			// controlled filenames from `git diff`. See VULN-91.
			return map[string]any{
				"modified_files": shellEscapeAll(tmplCtx.Steps.Changes.Modified...),
				"added_files":    shellEscapeAll(tmplCtx.Steps.Changes.Added...),
				"deleted_files":  shellEscapeAll(tmplCtx.Steps.Changes.Deleted...),
				"renamed_files":  shellEscapeAll(tmplCtx.Steps.Changes.Renamed...),
				"path":           tmplCtx.Steps.Path,
			}
		},
		// Leave batch_change_link alone; it will be rendered during the reconciler phase instead.
		"batch_change_link": func() string {
			return "${{ batch_change_link }}"
		},
	}
}

func RenderChangesetTemplateField(name, tmpl string, tmplCtx *ChangesetTemplateContext) (string, error) {
	var out bytes.Buffer

	// By default, text/template will continue even if it encounters a key that is not
	// indexed in any of the provided `FuncMap`s, replacing the variable with "<no
	// value>". This means that a mis-typed variable such as "${{
	// repository.search_resalt_paths }}" would just be evaluated as "<no value>", which
	// is not a particularly useful substitution and will only indirectly manifest to the
	// user as an error during execution. Instead, we prefer to fail immediately if we
	// encounter an unknown variable. We accomplish this by setting the option
	// "missingkey=error". See https://pkg.go.dev/text/template#Template.Option for more.
	t, err := New(name, tmpl, "missingkey=error", tmplCtx.ToFuncMap())
	if err != nil {
		return "", err
	}

	if err := t.Execute(&out, tmplCtx); err != nil {
		return "", err
	}

	return strings.TrimSpace(out.String()), nil
}
