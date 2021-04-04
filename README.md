# GELF Forwarder

Application that receives logs in formats not supported natively by Graylog server, forwarding them to Graylog via GELF TCP or UDP input.

## Features

- Supports multiple inputs (only one at a time, if you need more launch more instances)
  - HTTP
    - Supports both JSON array and whitespace delimited JSON (ndjson) formats
    - Body compression (gzip / zlib)
    - Healthcheck via GET/HEAD methods
    - Returns 429 response if message buffer is full
    - Works well with [vector's](https://vector.dev) `http` sink
  - Vector protobuf
    - Protocol used by [vector's](https://vector.dev) v0.12 `vector` sink
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
      --http-host-field string          Name of host field (default "host")
      --http-message-field string       Name of message field (default "message")
      --http-timestamp-field string     Name of timestamp field (default "timestamp")
      --input-type string               Which input to start: vector, http (default "http")
      --vector-address string           Listen address for vector input (default ":9000")
      --vector-host-field string        Name of host field (default "host")
      --vector-max-message-size uint    Maximum length of single Vector message (default 1048576)
      --vector-message-field string     Name of message field (default "message")
      --vector-timestamp-field string   Name of timestamp field (default "timestamp")
```

All options can be provided via flags or environment variables, for example:
```
INPUT_TYPE=vector ./gelf-forwarder --gelf-proto=tcp
```

Docker image is available on Dockerhub:
```
docker run --rm bslawianowski/gelf-forwarder:v0.1.0 --help
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
