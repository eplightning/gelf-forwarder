# GELF Forwarder

Application that receives logs in formats not supported natively by Graylog server, forwarding them to Graylog via GELF TCP or UDP input.

## Features

- Supports multiple inputs (only one at a time, if you need more launch more instances)
  - HTTP
    - Supports both JSON array and whitespace delimited JSON (ndjson) formats
    - Optional HTTP Basic authentication
    - Body compression (gzip / zlib)
    - Healthcheck via GET/HEAD methods
    - Returns 429 response if message buffer is full
    - Works well with [vector's](https://vector.dev) `http` sink
  - Vector protobuf (v1)
    - Protocol used by [vector's](https://vector.dev) v0.12 `vector` sink
  - Vector gRPC (v2)
    - Used by v0.15 `vector` sink with `version=2`
    - Not compatible with v0.14, please use older gelf-forwarder if you need it (`bslawianowski/gelf-forwarder:v0.2.0`)
- TLS support for serving server as well as client authentication
- Support for GELF output
  - TCP
  - UDP with optional compression
  - Additional fields are fully supported
- Basic backpressure and retry logic
  - If Graylog server is slow or not responding application will buffer up to `--channel-buffer-size` messages
  - Input will either decline messages (HTTP 429) or stop reading new messages (Vector input)
  - Exponential backoff for sending GELF messages with configurable number of retries via `--gelf-max-retries`
  - Graceful shutdown `--graceful-timeout`
## Usage

```
Usage of ./gelf-forwarder:
      --channel-buffer-size uint        How many messages to hold in channel buffer (default 100)
      --gelf-address string             Address of GELF server (default "127.0.0.1:12201")
      --gelf-compression                Enable compression for UDP (default true)
      --gelf-max-retries int            How many times to retry sending message in case of failure, -1 means infinity (default 3)
      --gelf-proto string               Protocol of GELf server (default "udp")
      --graceful-timeout uint           How many seconds to wait for messages to be sent on shutdown (default 10)
      --http-address string             Listen address for http input (default ":9000")
      --http-basic-pass string          Password for HTTP Basic authentication. Only used if username was set
      --http-basic-user string          Username for HTTP Basic authentication. Authentication is not required if empty (default)
      --http-host-field string          Name of host field (default "host")
      --http-message-field string       Name of message field (default "message")
      --http-timestamp-field string     Name of timestamp field (default "timestamp")
      --input-type string               Which input to start: vector, http, vectorv2 (default "http")
      --tls-cert-path string            Path to PEM-encoded certificate to be used for TLS server. Required if TLS was enabled
      --tls-client-ca-path string       Path to PEM-encoded CA bundle to be used for client certificate verification. When provided, TLS client authentication will be enabled and required
      --tls-enabled                     Use TLS for input
      --tls-key-path string             Path to PEM-encoded key to be used for TLS server. Required if TLS was enabled
      --vector-address string           Listen address for vector v1/v2 input (default ":9000")
      --vector-host-field string        Name of host field (default "host")
      --vector-max-message-size uint    Maximum length of single Vector v1 message (default 1048576)
      --vector-message-field string     Name of message field (default "message")
      --vector-timestamp-field string   Name of timestamp field (default "timestamp")
```

All options can be provided via flags or environment variables, for example:
```
INPUT_TYPE=vector ./gelf-forwarder --gelf-proto=tcp
```

Docker image is available on Github's GHCR:
```
docker run --rm ghcr.io/eplightning/gelf-forwarder:v0.4.1 --help
```

## Configuration tips

### Data format

Messages sent to `gelf-forwarder` need to contain at least two fields to work correctly:
- `message` - sent as `short_message` to GELF server
- `host` - sent as `host` to GELF server

Additionally `timestamp` should be provided.

In case of HTTP input the `timestamp` needs to be either unix timestamp or RFC3339 formatted date.

If `timestamp` is invalid or not provided the server will default to current time.

Names of the these special fields are fully configurable (see `--help`). They can't however be nested inside another object field.

### Authentication

All types of inputs support TLS client authentication, please refer to `--tls-*` family of options.

On top of that, HTTP input additionally supports HTTP basic authentication, please refer to `--http-basic-user` and `--http-basic-pass` options.


