FROM golang:1.15-alpine as builder

RUN apk add --update protoc protobuf-dev

COPY . /src
WORKDIR /src
RUN go generate ./... && go build ./cmd/marionette

FROM sourcegraph/src-campaign-volume-workspace

COPY --from=builder /src/marionette /usr/sbin/marionette
CMD ["/usr/sbin/marionette"]
