package util

import (
	"github.com/Graylog2/go-gelf/gelf"
	"regexp"
	"time"
)

var fieldRegex = regexp.MustCompile("\\W")

func NewGelfMessage() *gelf.Message {
	return &gelf.Message{
		Version:  "1.1",
		Extra:    make(map[string]interface{}),
		TimeUnix: float64(time.Now().UnixNano()) / float64(time.Second),
	}
}

func AppendExtraToGelf(msg *gelf.Message, key string, value interface{}) {
	msg.Extra["_" + fieldRegex.ReplaceAllString(key, "_")] = value
}

