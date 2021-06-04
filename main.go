package main

import (
	"github.com/Graylog2/go-gelf/gelf"
	"github.com/eplightning/gelf-forwarder/pkg/input"
	"github.com/eplightning/gelf-forwarder/pkg/output"
	"github.com/eplightning/gelf-forwarder/pkg/util"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

func main()  {
	pflag.String("input-type", "http", "Which input to start: vector, http, vectorv2")
	pflag.Uint("graceful-timeout", 10, "How many seconds to wait for messages to be sent on shutdown")
	pflag.Uint("channel-buffer-size", 100, "How many messages to hold in channel buffer")

	pflag.String("vector-address", ":9000", "Listen address for vector v1/v2 input")
	pflag.String("vector-timestamp-field", "timestamp", "Name of timestamp field")
	pflag.String("vector-message-field", "message", "Name of message field")
	pflag.String("vector-host-field", "host", "Name of host field")
	pflag.Uint("vector-max-message-size", input.DefaultMaxMessageSize, "Maximum length of single Vector v1 message")

	pflag.String("http-address", ":9000", "Listen address for http input")
	pflag.String("http-timestamp-field", "timestamp", "Name of timestamp field")
	pflag.String("http-message-field", "message", "Name of message field")
	pflag.String("http-host-field", "host", "Name of host field")

	pflag.String("gelf-address", "127.0.0.1:12201", "Address of GELF server")
	pflag.String("gelf-proto", "udp", "Protocol of GELf server")
	pflag.Int("gelf-max-retries", 3, "How many times to retry sending message in case of failure, -1 means infinity")
	pflag.Bool("gelf-compression", true, "Enable compression for UDP")

	pflag.Parse()

	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		panic("Could not initialize config")
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	logger, err := zap.NewProduction()
	if err != nil {
		panic("Could not initialize logging")
	}

	zap.ReplaceGlobals(logger)

	stopCh := make(chan interface{})
	msgCh := make(chan *gelf.Message, viper.GetUint("channel-buffer-size"))
	errCh := make(chan error, 2)
	wg := &sync.WaitGroup{}

	outOpts := output.NewGelfOutputOptions()
	outOpts.Address = viper.GetString("gelf-address")
	outOpts.GracefulTimeoutSeconds = viper.GetInt("graceful-timeout")
	outOpts.RetryLimit = viper.GetInt("gelf-max-retries")
	outOpts.Compression = viper.GetBool("gelf-compression")
	outOpts.Proto = viper.GetString("gelf-proto")
	out := output.NewGelfOutput(outOpts)

	var in util.Component

	switch viper.GetString("input-type") {
	case "vector":
		inOpts := input.NewVectorInputOptions()
		inOpts.Address = viper.GetString("vector-address")
		inOpts.MaxMsgSize = viper.GetUint32("vector-max-message-size")
		inOpts.HostField = viper.GetString("vector-host-field")
		inOpts.MessageField = viper.GetString("vector-message-field")
		inOpts.TimestampField = viper.GetString("vector-timestamp-field")
		in = input.NewVectorInput(inOpts)
	case "vectorv2":
		inOpts := input.NewVectorV2InputOptions()
		inOpts.Address = viper.GetString("vector-address")
		inOpts.HostField = viper.GetString("vector-host-field")
		inOpts.MessageField = viper.GetString("vector-message-field")
		inOpts.TimestampField = viper.GetString("vector-timestamp-field")
		in = input.NewVectorV2Input(inOpts)
	case "http":
		inOpts := input.NewHTTPInputOptions()
		inOpts.Address = viper.GetString("http-address")
		inOpts.HostField = viper.GetString("http-host-field")
		inOpts.MessageField = viper.GetString("http-message-field")
		inOpts.TimestampField = viper.GetString("http-timestamp-field")
		in = input.NewHTTPInput(inOpts)
	default:
		panic("invalid input type")
	}

	if err := util.RegisterComponent(in, wg, msgCh, stopCh, errCh); err != nil {
		zap.S().Panic("Could not start input", err)
	}
	if err := util.RegisterComponent(out, wg, msgCh, stopCh, errCh); err != nil {
		zap.S().Panic("Could not start output", err)
	}

	zap.S().Info("All components ready and listening")

	signals := make(chan os.Signal, 10)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		select {
		case err := <-errCh:
			zap.S().Error("One of the components failed, stopping ...", err)
		case <-signals:
			zap.S().Info("Received shutdown signal, stopping ...")
		}

		close(stopCh)
	}()

	wg.Wait()
}