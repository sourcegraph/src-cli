module github.com/sourcegraph/src-cli

go 1.13

require (
	github.com/Masterminds/semver v1.5.0
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.9.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/google/go-cmp v0.4.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/neelance/parallel v0.0.0-20160708114440-4de9ce63d14c
	github.com/olekukonko/tablewriter v0.0.4 // indirect
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/pkg/errors v0.9.1
	github.com/segmentio/textio v1.2.0
	github.com/sourcegraph/jsonx v0.0.0-20200625022044-c22a595bbad7
	github.com/ssor/bom v0.0.0-20170718123548-6386211fdfcf // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	golang.org/x/sys v0.0.0-20200622214017-ed371f2e16b4 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
	jaytaylor.com/html2text v0.0.0-20200412013138-3577fbdbcff7
)

replace github.com/gosuri/uilive v0.0.4 => github.com/mrnugget/uilive v0.0.4-fix-escape
