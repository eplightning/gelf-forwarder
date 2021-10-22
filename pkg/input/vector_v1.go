package input

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"

	"github.com/Graylog2/go-gelf/gelf"
	"github.com/eplightning/gelf-forwarder/pkg/util"
	vector "github.com/eplightning/gelf-forwarder/pkg/vector/event"
	"go.uber.org/zap"
)

const DefaultMaxMessageSize = 1 * 1024 * 1024

type VectorInput struct {
	address     string
	listener    net.Listener
	msgCh       chan *gelf.Message
	closed      bool
	connections *util.ConnectionMap
	schema      *vectorSchema
	maxMsgSize  uint32
	log         *zap.SugaredLogger
	tls         util.TLSInputOptions
}

type VectorInputOptions struct {
	Address        string
	TimestampField string
	MessageField   string
	HostField      string
	MaxMsgSize     uint32
	TLS            util.TLSInputOptions
}

func NewVectorInputOptions() VectorInputOptions {
	return VectorInputOptions{
		Address:        ":9000",
		TimestampField: "timestamp",
		MessageField:   "message",
		HostField:      "host",
		MaxMsgSize:     DefaultMaxMessageSize,
	}
}

func NewVectorInput(options VectorInputOptions) *VectorInput {
	return &VectorInput{
		address:     options.Address,
		connections: util.NewConnectionMap(),
		maxMsgSize:  options.MaxMsgSize,
		schema: &vectorSchema{
			timestampField: options.TimestampField,
			messageField:   options.MessageField,
			hostField:      options.HostField,
		},
		log: zap.S().With("component", "vector-input"),
		tls: options.TLS,
	}
}

func (v *VectorInput) Start() error {
	listener, err := net.Listen("tcp", v.address)
	if err != nil {
		return err
	}

	listener, err = util.WrapInputWithTLS(listener, v.tls)
	if err != nil {
		return err
	}

	v.listener = listener
	return nil
}

func (v *VectorInput) Listen(msgCh chan *gelf.Message, stopCh chan interface{}) error {
	v.msgCh = msgCh
	errCh := make(chan error)

	go v.acceptRoutine(errCh)

	v.log.Infof("Listening on %v", v.address)

	var err error
	select {
	case err = <-errCh:
	case <-stopCh:
		err = nil
	}

	v.log.Info("Closing connections")

	v.closed = true
	v.listener.Close()
	v.connections.CloseAll()

	return err
}

func (v *VectorInput) acceptRoutine(errCh chan error) {
	for {
		conn, err := v.listener.Accept()
		if err != nil {
			if v.closed {
				break
			} else {
				if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
					v.log.Warnf("Temporary error while accepting: %v", err)
					continue
				}

				errCh <- err
				break
			}
		}

		go v.readRoutine(conn)
	}
}

func (v *VectorInput) readRoutine(conn net.Conn) {
	id := v.connections.Add(conn)
	buf := make([]byte, v.maxMsgSize)

	v.log.Infof("Accepted connection #%v from %v", id, conn.RemoteAddr().String())

	for {
		_, err := io.ReadAtLeast(conn, buf[0:4], 4)
		if err != nil {
			v.log.Errorf("Unable to read message length, dropping connection: %v", err)
			v.connections.Close(id)
			return
		}

		var length uint32

		err = binary.Read(bytes.NewReader(buf), binary.BigEndian, &length)
		if err != nil || length > v.maxMsgSize {
			v.log.Errorf("Unable to decode message length, dropping connection: %v", err)
			v.connections.Close(id)
			return
		}

		_, err = io.ReadAtLeast(conn, buf[0:length], int(length))
		if err != nil {
			v.log.Errorf("Unable to read message, dropping connection: %v", err)
			v.connections.Close(id)
			return
		}

		event := &vector.EventWrapper{}
		if err = event.UnmarshalVT(buf[0:length]); err != nil {
			v.log.Errorf("Unable to decode message, ignoring: %v", err)
			continue
		}

		msg, err := v.schema.eventToGelf(event)
		if err != nil {
			v.log.Errorf("Unable to convert message to GELF, ignoring: %v", err)
			continue
		}

		v.msgCh <- msg
	}
}
