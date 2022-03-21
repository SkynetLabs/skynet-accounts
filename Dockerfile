FROM golang:1.18.0 as builder
LABEL maintainer="SkynetLabs <devs@skynetlabs.com>"

WORKDIR /root

ENV CGO_ENABLED=0

COPY . .

RUN go mod download && make release

FROM alpine:3.15.0
LABEL maintainer="SkynetLabs <devs@skynetlabs.com>"

COPY --from=builder /go/bin/skynet-accounts /usr/bin/skynet-accounts

ENTRYPOINT ["skynet-accounts"]
