FROM sourcegraph/alpine:3.10
COPY src /usr/bin/
ENTRYPOINT ["/usr/bin/src"]
