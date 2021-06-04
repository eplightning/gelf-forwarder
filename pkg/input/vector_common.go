package input

import (
	"fmt"
	"github.com/Graylog2/go-gelf/gelf"
	"github.com/eplightning/gelf-forwarder/pkg/util"
	vector "github.com/eplightning/gelf-forwarder/pkg/vector/event"
	"strconv"
	"strings"
	"time"
)

type vectorSchema struct {
	messageField   string
	hostField      string
	timestampField string
}

func (v *vectorSchema) eventToGelf(wrapper *vector.EventWrapper) (*gelf.Message, error) {
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
