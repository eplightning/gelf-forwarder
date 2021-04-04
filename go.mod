module github.com/eplightning/gelf-forwarder

go 1.16

require (
	github.com/Graylog2/go-gelf v0.0.0-20170811154226-7ebf4f536d8f
	github.com/cenkalti/backoff/v4 v4.1.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/valyala/fastjson v1.6.3
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.16.0
	google.golang.org/protobuf v1.26.0
)

replace github.com/Graylog2/go-gelf => ./third_party/go-gelf-2
