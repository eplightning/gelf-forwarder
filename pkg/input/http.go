package input

import (
	"compress/gzip"
	"compress/zlib"
	"crypto/subtle"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Graylog2/go-gelf/gelf"
	"github.com/eplightning/gelf-forwarder/pkg/util"
	"github.com/valyala/fastjson"
	"go.uber.org/zap"
)

type HTTPInput struct {
	address        string
	listener       net.Listener
	msgCh          chan *gelf.Message
	closed         bool
	timestampField string
	messageField   string
	hostField      string
	log            *zap.SugaredLogger
	basicUser      string
	basicPass      string
	tls            util.TLSInputOptions
	backpressure   bool
}

type HTTPInputOptions struct {
	Address        string
	TimestampField string
	MessageField   string
	HostField      string
	BasicUser      string
	BasicPass      string
	TLS            util.TLSInputOptions
	Backpressure   bool
}

func NewHTTPInputOptions() HTTPInputOptions {
	return HTTPInputOptions{
		Address:        ":9000",
		TimestampField: "timestamp",
		MessageField:   "message",
		HostField:      "host",
	}
}

func NewHTTPInput(options HTTPInputOptions) *HTTPInput {
	return &HTTPInput{
		address:        options.Address,
		timestampField: options.TimestampField,
		messageField:   options.MessageField,
		hostField:      options.HostField,
		basicUser:      options.BasicUser,
		basicPass:      options.BasicPass,
		log:            zap.S().With("component", "http-input"),
		tls:            options.TLS,
		backpressure:   options.Backpressure,
	}
}

func (h *HTTPInput) Start() error {
	listener, err := net.Listen("tcp", h.address)
	if err != nil {
		return err
	}

	listener, err = util.WrapInputWithTLS(listener, h.tls)
	if err != nil {
		return err
	}

	h.listener = listener
	return nil
}

func (h *HTTPInput) Listen(msgCh chan *gelf.Message, stopCh chan interface{}) error {
	h.msgCh = msgCh

	server := &http.Server{
		Addr:    h.address,
		Handler: h,
	}

	go func() {
		select {
		case <-stopCh:
			server.Close()
		}
	}()

	h.log.Infof("Listening on %v", h.address)

	if err := server.Serve(h.listener); err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (h *HTTPInput) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.Method == "GET" || req.Method == "HEAD" || req.Method == "OPTIONS" {
		writer.WriteHeader(http.StatusOK)
		return
	}

	if !h.authenticate(writer, req) {
		return
	}

	msgs, err := h.readMessages(req)
	if err != nil {
		h.log.Errorf("Unable to read messages from HTTP request: %v", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(msgs) == 0 {
		h.log.Warnf("Received 0 messages")
	}

	if h.backpressure && len(msgs)+len(h.msgCh) > cap(h.msgCh) {
		writer.WriteHeader(http.StatusTooManyRequests)
		return
	}

	for _, msg := range msgs {
		h.msgCh <- msg
	}

	writer.WriteHeader(http.StatusOK)
}

func (h *HTTPInput) authenticate(writer http.ResponseWriter, req *http.Request) bool {
	if h.basicUser != "" {
		user, pass, ok := req.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(h.basicUser)) == 0 || subtle.ConstantTimeCompare([]byte(pass), []byte(h.basicPass)) == 0 {
			writer.Header().Set("WWW-Authenticate", `Basic realm="gelf-forwarder"`)
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)

			return false
		}
	}

	return true
}

func (h *HTTPInput) readMessages(req *http.Request) ([]*gelf.Message, error) {
	var body io.Reader
	var err error

	switch req.Header.Get("content-encoding") {
	case "gzip":
		body, err = gzip.NewReader(req.Body)
	case "deflate":
		body, err = zlib.NewReader(req.Body)
	default:
		body = req.Body
	}

	if err != nil {
		return nil, err
	}

	// currently JSON is assumed: [{msg},{msg2}] and {msg}\n{msg2}
	return h.parseJSON(body)
}

func (h *HTTPInput) parseJSON(br io.Reader) ([]*gelf.Message, error) {
	body, err := ioutil.ReadAll(br)
	if err != nil {
		return nil, err
	}

	var sc fastjson.Scanner
	sc.InitBytes(body)

	var msgs []*gelf.Message

	for sc.Next() {
		value := sc.Value()
		switch t := value.Type(); t {
		case fastjson.TypeObject:
			obj, _ := value.Object()
			msg, err := h.jsonObjectToMessage(obj)
			if err != nil {
				h.log.Warnf("Unable to create message from JSON object: %v", err)
				continue
			}
			msgs = append(msgs, msg)
		case fastjson.TypeArray:
			arr, _ := value.Array()
			for _, item := range arr {
				obj, err := item.Object()
				if err != nil {
					h.log.Warnf("Expected object inside array: %v", err)
					continue
				}
				msg, err := h.jsonObjectToMessage(obj)
				if err != nil {
					h.log.Warnf("Unable to create message from JSON object: %v", err)
					continue
				}
				msgs = append(msgs, msg)
			}
		default:
			h.log.Warnf("Ignoring %v, not an array or object", t)
		}
	}
	if err = sc.Error(); err != nil {
		return nil, err
	}

	return msgs, nil
}

func (h *HTTPInput) jsonObjectToMessage(obj *fastjson.Object) (*gelf.Message, error) {
	out := util.NewGelfMessage()

	// short_message
	msg, err := requireJsonString(obj.Get(h.messageField))
	if err != nil {
		return nil, fmt.Errorf("error while setting short_message: %v", err)
	}
	obj.Del(h.messageField)
	out.Short = msg

	// host
	host, err := requireJsonString(obj.Get(h.hostField))
	if err != nil {
		return nil, fmt.Errorf("error while setting host: %v", err)
	}
	obj.Del(h.hostField)
	out.Host = host

	// timestamp
	tsRaw := obj.Get(h.timestampField)
	if tsRaw != nil {
		ts, err := jsonValueToUnixTimestamp(tsRaw)
		if err != nil {
			h.log.Warnf("Unable to parse timestamp: %v", err)
		} else {
			out.TimeUnix = ts
		}

		obj.Del(h.timestampField)
	}

	obj.Visit(func(key []byte, v *fastjson.Value) {
		processJsonExtra(out, string(key), v)
	})

	return out, nil
}

func jsonValueToUnixTimestamp(value *fastjson.Value) (float64, error) {
	switch t := value.Type(); t {
	case fastjson.TypeString:
		str, _ := value.StringBytes()
		ts, err := time.Parse(time.RFC3339Nano, string(str))
		if err != nil {
			return 0, err
		}
		return float64(ts.UnixNano()) / float64(time.Second), nil
	case fastjson.TypeNumber:
		float, _ := value.Float64()
		return float, nil
	default:
		return 0, fmt.Errorf("unexpected type: %v", t)
	}
}

func jsonValueToString(value *fastjson.Value) string {
	str, err := value.StringBytes()
	if err != nil {
		return value.String()
	}

	return string(str)
}

func requireJsonString(field *fastjson.Value) (string, error) {
	if field == nil {
		return "", fmt.Errorf("field doesn't exist")
	}

	str := jsonValueToString(field)
	if len(strings.TrimSpace(str)) == 0 {
		return "", fmt.Errorf("field is empty")
	}

	return str, nil
}

func processJsonExtra(msg *gelf.Message, key string, value *fastjson.Value) {
	switch value.Type() {
	case fastjson.TypeNumber:
		num, _ := value.Float64()
		util.AppendExtraToGelf(msg, key, num)
	case fastjson.TypeObject:
		obj, _ := value.Object()
		obj.Visit(func(subk []byte, subv *fastjson.Value) {
			newKey := key + "_" + string(subk)
			processJsonExtra(msg, newKey, subv)
		})
	case fastjson.TypeArray:
		arr, _ := value.Array()
		for i, subv := range arr {
			newKey := key + "_" + strconv.FormatInt(int64(i), 10)
			processJsonExtra(msg, newKey, subv)
		}
	default:
		util.AppendExtraToGelf(msg, key, jsonValueToString(value))
	}
}
