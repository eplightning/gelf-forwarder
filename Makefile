default: gelf-forwarder

gelf-forwarder:
	CGO_ENABLED=0 go build -ldflags "-s -w" .

clean:
	rm -f gelf-forwarder

proto:
	protoc --proto_path=proto --go_opt=Mvector/event.proto=github.com/eplightning/gelf-forwarder/pkg/vector --go_out=. --go_opt=module=github.com/eplightning/gelf-forwarder vector/event.proto

.PHONY: default clean proto