FROM golang:1.17-alpine AS builder

RUN apk update && apk add --no-cache git make

WORKDIR /src
COPY . .

RUN make

FROM alpine:3.13

COPY --from=builder /src/gelf-forwarder /usr/bin/gelf-forwarder

USER 999

ENTRYPOINT ["/usr/bin/gelf-forwarder"]
