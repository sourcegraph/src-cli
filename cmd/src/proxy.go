package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/srcproxy"
)

func init() {
	// NOTE: this is an experimental command. It isn't advertised in -help

	flagSet := flag.NewFlagSet("proxy", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), `'src proxy' starts a local reverse proxy to your Sourcegraph instance.

USAGE
  src [-v] proxy [-addr :7777] [-insecure-skip-verify] [-server-cert cert.pem -server-key key.pem] [-log-file path] [client-ca.pem]

By default, proxied requests use SRC_ACCESS_TOKEN via:
  Authorization: token SRC_ACCESS_TOKEN

If a client CA certificate path is provided, proxy runs in mTLS sudo mode:
  1. Serves HTTPS and requires a client certificate signed by the provided CA.
  2. Reads the first email SAN from the presented client certificate.
  3. Looks up the Sourcegraph user by that email.
  4. Proxies requests with:
     Authorization: token-sudo token="TOKEN",user="USERNAME"

Server certificate options:
  -server-cert and -server-key can be used to provide the TLS certificate
  and key used by the local proxy server. If omitted in cert mode, an
  ephemeral self-signed server certificate is generated.
`)
	}

	var (
		addrFlag               = flagSet.String("addr", ":7777", "Address on which to serve")
		insecureSkipVerifyFlag = flagSet.Bool("insecure-skip-verify", false, "Skip validation of TLS certificates against trusted chains")
		serverCertFlag         = flagSet.String("server-cert", "", "Path to TLS server certificate for local proxy listener")
		serverKeyFlag          = flagSet.String("server-key", "", "Path to TLS server private key for local proxy listener")
		logFileFlag            = flagSet.String("log-file", "", "Path to log file. If not set, logs are written to stderr")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		var clientCAPath string
		switch flagSet.NArg() {
		case 0:
		case 1:
			clientCAPath = flagSet.Arg(0)
		default:
			return cmderrors.Usage("requires zero or one positional argument: path to client CA certificate")
		}
		if (*serverCertFlag == "") != (*serverKeyFlag == "") {
			return cmderrors.Usage("both -server-cert and -server-key must be provided together")
		}

		logOutput := io.Writer(os.Stderr)
		var logF *os.File
		if *logFileFlag != "" {
			var err error
			logF, err = os.Create(*logFileFlag)
			if err != nil {
				return errors.Wrap(err, "open log file")
			}
			defer func() { _ = logF.Close() }()
			logOutput = logF
		}

		dbug := log.New(io.Discard, "", log.LstdFlags)
		if *verbose {
			dbug = log.New(logOutput, "DBUG proxy: ", log.LstdFlags)
		}

		s := &srcproxy.Serve{
			Addr:               *addrFlag,
			Endpoint:           cfg.Endpoint,
			AccessToken:        cfg.AccessToken,
			ClientCAPath:       clientCAPath,
			ServerCertPath:     *serverCertFlag,
			ServerKeyPath:      *serverKeyFlag,
			InsecureSkipVerify: *insecureSkipVerifyFlag,
			AdditionalHeaders:  cfg.AdditionalHeaders,
			Info:               log.New(logOutput, "proxy: ", log.LstdFlags),
			Debug:              dbug,
		}
		return s.Start()
	}

	commands = append(commands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
