FROM sourcegraph/alpine:3.10

# needed for `src lsif upload`
RUN apk add --no-cache git

COPY src /usr/bin/
ENTRYPOINT ["/usr/bin/src"]
