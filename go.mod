module github.com/sourcegraph/src-cli

go 1.13

require (
	github.com/Masterminds/semver v1.5.0
	github.com/apex/log v1.9.0 // indirect
	github.com/bradfitz/iter v0.0.0-20191230175014-e8f45d346db8 // indirect
	github.com/c4milo/unpackit v0.0.0-20170704181138-4ed373e9ef1c // indirect
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/efritz/pentimento v0.0.0-20190429011147-ade47d831101
	github.com/gobwas/glob v0.2.3
	github.com/google/go-cmp v0.5.2
	github.com/google/go-github v17.0.0+incompatible // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/gosuri/uilive v0.0.4 // indirect
	github.com/gosuri/uiprogress v0.0.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hooklift/assert v0.1.0 // indirect
	github.com/jig/teereadcloser v0.0.0-20181016160506-953720c48e05
	github.com/json-iterator/go v1.1.10
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-runewidth v0.0.9
	github.com/neelance/parallel v0.0.0-20160708114440-4de9ce63d14c
	github.com/nsf/termbox-go v0.0.0-20200418040025-38ba6e5628f1
	github.com/olekukonko/tablewriter v0.0.4 // indirect
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/pkg/errors v0.9.1
	github.com/sourcegraph/campaignutils v0.0.0-20201124055807-2f9cfa9317e2
	github.com/sourcegraph/codeintelutils v0.0.0-20210118231003-6698e102a8a1
	github.com/sourcegraph/go-diff v0.6.1
	github.com/sourcegraph/jsonx v0.0.0-20200629203448-1a936bd500cf
	github.com/ssor/bom v0.0.0-20170718123548-6386211fdfcf // indirect
	github.com/tj/go-update v2.2.4+incompatible
	github.com/ulikunitz/xz v0.5.9 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20210113181707-4bcb84eeeb78
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	jaytaylor.com/html2text v0.0.0-20200412013138-3577fbdbcff7
)

replace github.com/gosuri/uilive v0.0.4 => github.com/mrnugget/uilive v0.0.4-fix-escape

// See: https://github.com/ghodss/yaml/pull/65
replace github.com/ghodss/yaml => github.com/sourcegraph/yaml v1.0.1-0.20200714132230-56936252f152
