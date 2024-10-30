#!/usr/bin/env bash

# A test script to help manually test permutations of the proxy settings.
# Before running this script,you'll need five terminal windows:
# One for `socat`, two for `mitmproxy` (non-auth  and auth),
# and two for a socks proxy (none-auth and auth).
# If you're on macOS, run `brew install socat mitmproxy`.
# To avoid the need to use insecure TLS settings, import mitmproxy's CA certificate into your keychain.
# On macOS: `sudo security add-trusted-cert -d -p ssl -p basic -k /Library/Keychains/System.keychain ~/.mitmproxy/mitmproxy-ca-cert.pem`
# For a socks proxy, I use the Docker image serjs/go-socks5-proxy.
# Note that `mitmproxy` is not a tunneling proxy - it uses a self-signed certificate
# to authenticate clients, which is good and bad. Good because we can inspect requests coming through
# if we want, and bad because we need to use insecure TLS settings to use it.
# It also works with `socat`, where `tinyproxy` and even `devproxy.sgdev.org` don't.
# Terminal 1:
# `socat -d -d unix-listen:${HOME}/socat-proxy.sock,fork tcp:localhost:8080`
# Terminal 2:
# `mitmproxy -v -p 8080`
# Terminal 3:
# `mitmproxy -v -p 8081 --proxyauth user:pass`
# Terminal 4:
# `docker run --rm -p 1080:1080 serjs/go-socks5-proxy`
# Terminal 5:
# `docker run --rm -p 1081:1080 -e PROXY_USER=user -e PROXY_PASSWORD=pass serjs/go-socks5-proxy`
# when you kick off this script, keep all of the windows in view so you can see that the output
# shows successful connections being made.

SRC_PATH=${SRC_PATH:-~/go/bin/src}

export SRC_ENDPOINT=${SRC_ENDPOINT:-https://sourcegraph.com}
export SRC_ACCESS_TOKEN=${SRC_ACCESS_TOKEN}

socket=~/socat-proxy.sock

# UNIX Domain Socket test
# You should see connection output in both the `socat` and `mitmproxy` terminals.
echo "UNIX Domain Socket test"
SRC_PROXY=${socket} \
${SRC_PATH} login

# HTTP test
# You should see connection output in the `mitmproxy` terminal.
echo "HTTP proxy test"
SRC_PROXY=http://localhost:8080 \
${SRC_PATH} login

# HTTPS with auth test
# You should see connection output in the `mitmproxy` with auth terminal.
echo "HTTP proxy with auth test"
SRC_PROXY=http://user:pass@localhost:8081 \
${SRC_PATH} login

# HTTPS test
# You should see connection output in the `mitmproxy` terminal.
echo "HTTPS proxy test"
SRC_PROXY=https://localhost:8080 \
${SRC_PATH} login

# HTTPS with auth test
# You should see connection output in the `mitmproxy` with auth terminal.
echo "HTTPS proxy with auth test"
SRC_PROXY=https://user:pass@localhost:8081 \
${SRC_PATH} login

# SOCKS test
# You should see connection output in the socks terminal.
echo "SOCKS proxy test"
SRC_PROXY=socks5://localhost:1080 \
${SRC_PATH} login

# SOCKS with auth test
# You should see connection output in the socks terminal.
echo "SOCKS proxy with auth test"
SRC_PROXY=socks5://user:pass@localhost:1081 \
${SRC_PATH} login

# HTTPS using insecure TLS code path
# You should see connection output in the `mitmproxy` terminal.
echo "HTTPS proxy insecure TLS path test"
SRC_PROXY=https://localhost:8080 \
${SRC_PATH} login --insecure-skip-verify=true


# test a search using a proxy
echo "Search test"
SRC_PROXY=https://localhost:8080 \
${SRC_PATH} search -json 'repo:github.com/sourcegraph/src-cli foobar'


# test with the system proxy set to something else
echo "Ignoring system proxy test"
https_proxy=http://localhost:12345 \
SRC_PROXY=https://localhost:8080 \
${SRC_PATH} login
