package output

import (
	"context"
	"fmt"
	"github.com/Graylog2/go-gelf/gelf"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
	"time"
)

type GelfOutput struct {
	proto           string
	address         string
	compression     bool
	retryLimit      int
	gracefulTimeout time.Duration
	writer          gelf.Writer
	log             *zap.SugaredLogger
}

type GelfOutputOptions struct {
	Proto                  string
	Address                string
	Compression            bool
	RetryLimit             int
	GracefulTimeoutSeconds int
}

func NewGelfOutputOptions() GelfOutputOptions {
	return GelfOutputOptions{
		Proto:                  "udp",
		Address:                "127.0.0.1:12201",
		Compression:            true,
		RetryLimit:             3,
		GracefulTimeoutSeconds: 10,
	}
}

func NewGelfOutput(options GelfOutputOptions) *GelfOutput {
	return &GelfOutput{
		proto:           options.Proto,
		address:         options.Address,
		compression:     options.Compression,
		retryLimit:      options.RetryLimit,
		gracefulTimeout: time.Duration(options.GracefulTimeoutSeconds) * time.Second,
		log:             zap.S().With("component", "gelf-output"),
	}
}

func (o *GelfOutput) Start() error {
	switch o.proto {
	case "tcp":
		writer, err := gelf.NewTCPWriter(o.address)
		if err != nil {
			return fmt.Errorf("unable to initialize TCP GELF writer: %v", err)
		}
		o.writer = writer
	case "udp":
		writer, err := gelf.NewUDPWriter(o.address, o.compression)
		if err != nil {
			return fmt.Errorf("unable to initialize UDP GELF writer: %v", err)
		}
		o.writer = writer
	}

	return nil
}

func (o *GelfOutput) Listen(msgCh chan *gelf.Message, stopCh chan interface{}) error {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-stopCh:
			cancel()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			o.gracefulStop(msgCh)
			return nil
		case msg := <-msgCh:
			if err := o.send(ctx, msg); err != nil {
				o.log.Errorf("Max attempts reached, dropping: %v", err)
			}
		}
	}
}

func (o *GelfOutput) gracefulStop(msgCh chan *gelf.Message) {
	o.log.Infof("Graceful shutdown initiated, forcing shutdown after %v", o.gracefulTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), o.gracefulTimeout)

	for {
		select {
		case <-ctx.Done():
			if remaining := len(msgCh); remaining > 0 {
				o.log.Warnf("Forcing shutdown with %v messages unsent", remaining)
			}
			cancel()
			return
		case msg := <-msgCh:
			if err := o.send(ctx, msg); err != nil {
				o.log.Errorf("Max attempts reached, dropping: %v", err)
			}
		default:
			cancel()
			return
		}
	}
}

func (o *GelfOutput) send(ctx context.Context, msg *gelf.Message) error {
	var bo backoff.BackOff = backoff.WithContext(backoff.NewExponentialBackOff(), ctx)

	if o.retryLimit > -1 {
		bo = backoff.WithMaxRetries(bo, uint64(o.retryLimit))
	}

	operation := func() error {
		err := o.writer.WriteMessage(msg)
		if err != nil {
			o.log.Warnf("Error while writing GELF message: %v", err)
			return fmt.Errorf("error while writing GELF message: %v", err)
		}

		return nil
	}

	return backoff.Retry(operation, bo)
}
