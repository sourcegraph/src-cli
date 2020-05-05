module github.com/sourcegraph/src-cli

go 1.13

require (
	github.com/Masterminds/semver v1.5.0
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.9.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/gosuri/uilive v0.0.4
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
	github.com/sourcegraph/go-diff v0.5.1
	github.com/sourcegraph/jsonx v0.0.0-20190114210550-ba8cb36a8614
	github.com/ssor/bom v0.0.0-20170718123548-6386211fdfcf // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/net v0.0.0-20200505041828-1ed23360d12c
	golang.org/x/sys v0.0.0-20200501145240-bc7a7d42d5c3 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
	jaytaylor.com/html2text v0.0.0-20200412013138-3577fbdbcff7
	sourcegraph.com/sqs/pbtypes v1.0.0 // indirect
)

replace github.com/gosuri/uilive v0.0.4 => github.com/mrnugget/uilive v0.0.4-fix-escape
