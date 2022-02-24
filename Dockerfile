FROM golang:1.17.7 as builder
LABEL maintainer="SkynetLabs <devs@skynetlabs.com>"

WORKDIR /root

ENV CGO_ENABLED=0

COPY api api
COPY build build
COPY database database
COPY email email
COPY hash hash
COPY jwt jwt
COPY lib lib
COPY metafetcher metafetcher
COPY skynet skynet
COPY go.mod go.sum main.go Makefile ./

RUN go mod download && make release

FROM alpine:3.15.0
LABEL maintainer="SkynetLabs <devs@skynetlabs.com>"

COPY --from=builder /go/bin/skynet-accounts /usr/bin/skynet-accounts

ENTRYPOINT ["skynet-accounts"]
