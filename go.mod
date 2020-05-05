module github.com/sourcegraph/src-cli

go 1.13

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/Masterminds/semver v1.5.0
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/checkpoint-restore/go-criu v0.0.0-20191125063657-fcdcd07065c5 // indirect
	github.com/cilium/ebpf v0.0.0-20200220135754-243d06f69c38 // indirect
	github.com/containerd/console v0.0.0-20191219165238-8375c3424e4d // indirect
	github.com/containerd/containerd v1.3.4 // indirect
	github.com/containerd/continuity v0.0.0-20200107194136-26c1120b8d41 // indirect
	github.com/containerd/fifo v0.0.0-20191213151349-ff969a566b00 // indirect
	github.com/coreos/etcd v3.3.18+incompatible // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/cyphar/filepath-securejoin v0.2.2 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.4.2-0.20200221230207-d706420b5d0f
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/swarmkit v1.12.1-0.20200128161603-49e35619b182 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.9.0
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/protobuf v1.4.0 // indirect
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/gosuri/uilive v0.0.4
	github.com/graph-gophers/graphql-go v0.0.0-20200207002730-8334863f2c8b
	github.com/hashicorp/go-memdb v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/mattn/go-shellwords v1.0.10 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mrunalp/fileutils v0.0.0-20171103030105-7d4729fb3618 // indirect
	github.com/neelance/parallel v0.0.0-20160708114440-4de9ce63d14c
	github.com/olekukonko/tablewriter v0.0.4 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v1.0.0-rc9.0.20200221051241-688cf6d43cc4 // indirect
	github.com/opencontainers/runtime-spec v1.0.1 // indirect
	github.com/opencontainers/selinux v1.3.2 // indirect
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.4.1 // indirect
	github.com/prometheus/procfs v0.0.10 // indirect
	github.com/seccomp/libseccomp-golang v0.9.1 // indirect
	github.com/segmentio/textio v1.2.0
	github.com/sirupsen/logrus v1.5.0 // indirect
	github.com/sourcegraph/go-diff v0.5.1
	github.com/sourcegraph/jsonx v0.0.0-20190114210550-ba8cb36a8614
	github.com/ssor/bom v0.0.0-20170718123548-6386211fdfcf // indirect
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2 // indirect
	github.com/urfave/cli v1.22.2 // indirect
	github.com/vbatts/tar-split v0.11.1 // indirect
	github.com/vishvananda/netlink v1.1.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/net v0.0.0-20200425230154-ff2c4b7c35a0
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/genproto v0.0.0-20200424135956-bca184e23272 // indirect
	google.golang.org/grpc v1.29.1 // indirect
	gotest.tools/v3 v3.0.2 // indirect
	jaytaylor.com/html2text v0.0.0-20200412013138-3577fbdbcff7
	sourcegraph.com/sqs/pbtypes v1.0.0 // indirect
)

replace github.com/gosuri/uilive v0.0.4 => github.com/mrnugget/uilive v0.0.4-fix-escape

// replace github.com/docker/docker v1.4.2-0.20200221230207-d706420b5d0f => github.com/docker/engine v1.4.2-0.20200221230207-d706420b5d0f
