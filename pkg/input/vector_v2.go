package input

import (
	"context"
	"net"

	"github.com/Graylog2/go-gelf/gelf"
	"github.com/eplightning/gelf-forwarder/pkg/util"
	"github.com/eplightning/gelf-forwarder/pkg/vector/api"
	vtgrpc "github.com/planetscale/vtprotobuf/codec/grpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/proto"
)

func init() {
	encoding.RegisterCodec(vtgrpc.Codec{})
}

type VectorV2Input struct {
	api.UnimplementedVectorServer
	address  string
	listener net.Listener
	msgCh    chan *gelf.Message
	schema   *vectorSchema
	log      *zap.SugaredLogger
	server   *grpc.Server
	tls      util.TLSInputOptions
}

type VectorV2InputOptions struct {
	Address        string
	TimestampField string
	MessageField   string
	HostField      string
	TLS            util.TLSInputOptions
}

func NewVectorV2InputOptions() VectorV2InputOptions {
	return VectorV2InputOptions{
		Address:        ":9000",
		TimestampField: "timestamp",
		MessageField:   "message",
		HostField:      "host",
	}
}

func NewVectorV2Input(options VectorV2InputOptions) *VectorV2Input {
	return &VectorV2Input{
		address: options.Address,
		schema: &vectorSchema{
			timestampField: options.TimestampField,
			messageField:   options.MessageField,
			hostField:      options.HostField,
		},
		log: zap.S().With("component", "vector-v2-input"),
		tls: options.TLS,
	}
}

func (v *VectorV2Input) Start() error {
	listener, err := net.Listen("tcp", v.address)
	if err != nil {
		return err
	}

	listener, err = util.WrapInputWithTLS(listener, v.tls)
	if err != nil {
		return err
	}

	v.listener = listener

	v.server = grpc.NewServer()
	api.RegisterVectorServer(v.server, v)

	return nil
}

func (v *VectorV2Input) Listen(msgCh chan *gelf.Message, stopCh chan interface{}) error {
	v.msgCh = msgCh

	go func() {
		select {
		case <-stopCh:
			v.log.Info("Stopping gRPC server")
			v.server.Stop()
		}
	}()

	v.log.Infof("Listening on %v", v.address)

	return v.server.Serve(v.listener)
}

func (v *VectorV2Input) PushEvents(ctx context.Context, req *api.PushEventsRequest) (*api.PushEventsResponse, error) {
	// TODO: for now this will always block if full, should we error out instead?
	for _, e := range req.Events {
		msg, err := v.schema.eventToGelf(e)
		if err != nil {
			v.log.Errorf("Unable to convert message to GELF, ignoring: %v", err)
		} else {
			v.msgCh <- msg
		}
	}

	return &api.PushEventsResponse{}, nil
}

func (v *VectorV2Input) HealthCheck(ctx context.Context, req *api.HealthCheckRequest) (
	*api.HealthCheckResponse, error,
) {
	return &api.HealthCheckResponse{Status: api.ServingStatus_SERVING}, nil
}
