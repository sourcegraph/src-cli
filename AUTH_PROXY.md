# Authenticating requests behind a proxy

If your instance is behind an authenticating proxy that requires additional headers, these can be supplied via environment variables:

```sh
SRC_HEADER_NAME=value src search 'foobar'
```

In this example, the header name-value pair `Name: value` will be threaded to all HTTP requests to your instance. Multiple such headers can be supplied.
