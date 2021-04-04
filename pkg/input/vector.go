package input

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/Graylog2/go-gelf/gelf"
	"github.com/eplightning/gelf-forwarder/pkg/util"
	"github.com/eplightning/gelf-forwarder/pkg/vector"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const DefaultMaxMessageSize = 1 * 1024 * 1024

type VectorInput struct {
	address        string
	listener       net.Listener
	msgCh          chan *gelf.Message
	closed         bool
	connections    *util.ConnectionMap
	timestampField string
	messageField   string
	hostField      string
	maxMsgSize     uint32
	log            *zap.SugaredLogger
}

type VectorInputOptions struct {
	Address        string
	TimestampField string
	MessageField   string
	HostField      string
	MaxMsgSize     uint32
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
		address:        options.Address,
		connections:    util.NewConnectionMap(),
		timestampField: options.TimestampField,
		messageField:   options.MessageField,
		hostField:      options.HostField,
		maxMsgSize:     options.MaxMsgSize,
		log:            zap.S().With("component", "vector-input"),
	}
}

func (v *VectorInput) Start() error {
	listener, err := net.Listen("tcp", v.address)
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
		if err = proto.Unmarshal(buf[0:length], event); err != nil {
			v.log.Errorf("Unable to decode message, ignoring: %v", err)
			continue
		}

		msg, err := v.eventToGelf(event)
		if err != nil {
			v.log.Errorf("Unable to convert message to GELF, ignoring: %v", err)
			continue
		}

		v.msgCh <- msg
	}
}

func (v *VectorInput) eventToGelf(wrapper *vector.EventWrapper) (*gelf.Message, error) {
	log := wrapper.GetLog()
	if log == nil {
		return nil, fmt.Errorf("metrics are not supported")
	}

	out := util.NewGelfMessage()

	// short_message
	msg, err := requireString(log.Fields[v.messageField])
	if err != nil {
		return nil, fmt.Errorf("error while setting short_message: %v", err)
	}
	delete(log.Fields, v.messageField)
	out.Short = msg

	// host
	host, err := requireString(log.Fields[v.hostField])
	if err != nil {
		return nil, fmt.Errorf("error while setting host: %v", err)
	}
	delete(log.Fields, v.hostField)
	out.Host = host

	// timestamp
	tsRaw, exists := log.Fields[v.timestampField]
	if exists {
		delete(log.Fields, v.timestampField)

		ts := tsRaw.GetTimestamp()
		if ts != nil {
			out.TimeUnix = float64(ts.AsTime().UnixNano()) / float64(time.Second)
		}
	}

	for k, v := range log.GetFields() {
		processExtra(out, k, v)
	}

	return out, nil
}

func requireString(field *vector.Value) (string, error) {
	if field == nil {
		return "", fmt.Errorf("field doesn't exist")
	}

	str := vectorValueToString(field)
	if len(strings.TrimSpace(str)) == 0 {
		return "", fmt.Errorf("field is empty")
	}

	return str, nil
}

func processExtra(msg *gelf.Message, key string, value *vector.Value) {
	switch casted := value.Kind.(type) {
	case *vector.Value_Integer:
		util.AppendExtraToGelf(msg, key, casted.Integer)
	case *vector.Value_Float:
		util.AppendExtraToGelf(msg, key, casted.Float)
	case *vector.Value_Map:
		for subk, subv := range casted.Map.Fields {
			newKey := key + "_" + subk
			processExtra(msg, newKey, subv)
		}
	case *vector.Value_Array:
		for i, subv := range casted.Array.Items {
			newKey := key + "_" + strconv.FormatInt(int64(i), 10)
			processExtra(msg, newKey, subv)
		}
	default:
		util.AppendExtraToGelf(msg, key, vectorValueToString(value))
	}
}

func vectorValueToString(value *vector.Value) string {
	switch casted := value.Kind.(type) {
	case *vector.Value_RawBytes:
		return string(casted.RawBytes)
	case *vector.Value_Integer:
		return strconv.FormatInt(casted.Integer, 10)
	case *vector.Value_Float:
		return strconv.FormatFloat(casted.Float, 'f', -1, 64)
	case *vector.Value_Timestamp:
		return casted.Timestamp.AsTime().Format(time.RFC3339Nano)
	case *vector.Value_Boolean:
		return strconv.FormatBool(casted.Boolean)
	case *vector.Value_Null:
		return "null"
	default:
		return value.String()
	}
}
