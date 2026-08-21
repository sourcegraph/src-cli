module github.com/sourcegraph/sourcegraph/lib

go 1.26.5

require (
	github.com/Masterminds/semver v1.5.0
	github.com/charmbracelet/glamour v1.0.0
	github.com/charmbracelet/lipgloss v1.1.1-0.20250404203927-76690c660834
	github.com/cockroachdb/errors v1.14.0
	github.com/cockroachdb/redact v1.1.8
	github.com/ghodss/yaml v1.0.0
	github.com/gobwas/glob v0.2.3
	github.com/google/go-cmp v0.7.0
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853
	github.com/jackc/pgx/v5 v5.10.0
	github.com/klauspost/pgzip v1.2.6
	github.com/mattn/go-isatty v0.0.24
	github.com/mattn/go-runewidth v0.0.28
	github.com/moby/term v0.5.2
	github.com/muesli/termenv v0.16.0
	github.com/scip-code/scip/bindings/go/scip v0.9.0
	github.com/sourcegraph/conc v0.3.0
	github.com/sourcegraph/go-diff v0.8.0
	github.com/sourcegraph/log v0.0.0-20260714115934-b104ff56b299
	github.com/stretchr/testify v1.11.1
	github.com/urfave/cli/v3 v3.11.0
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/xlab/treeprint v1.2.0
	go.opentelemetry.io/otel v1.45.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/alecthomas/chroma/v2 v2.27.0 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/exp/slice v0.0.0-20260816001655-68d539dca504 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cockroachdb/logtags v0.0.0-20241215232642-bb51bb14a506 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2/v2 v2.7.1 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/getsentry/sentry-go v0.48.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/sourcegraph/beaut v0.0.0-20240611013027-627e4c25335a // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xo/terminfo v1.0.0 // indirect
	github.com/yuin/goldmark v1.8.5 // indirect
	github.com/yuin/goldmark-emoji v1.0.6 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// See: https://github.com/ghodss/yaml/pull/65
replace github.com/ghodss/yaml => github.com/sourcegraph/yaml v1.0.1-0.20200714132230-56936252f152
