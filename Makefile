default: gelf-forwarder

gelf-forwarder:
	CGO_ENABLED=0 go build -ldflags "-s -w" .

clean:
	rm -f gelf-forwarder

proto:
	protoc --proto_path=proto/vector \
		--go-vtproto_opt=features=marshal+unmarshal+size \
		--go_opt=module=github.com/eplightning/gelf-forwarder \
		--go-grpc_opt=module=github.com/eplightning/gelf-forwarder \
		--go-vtproto_opt=module=github.com/eplightning/gelf-forwarder \
		--go_out=. --go-vtproto_out=. --go-grpc_out=. vector.proto

	protoc --proto_path=proto/vector \
		--go-vtproto_opt=features=marshal+unmarshal+size \
		--go_opt=module=github.com/eplightning/gelf-forwarder \
		--go-grpc_opt=module=github.com/eplightning/gelf-forwarder \
		--go-vtproto_opt=module=github.com/eplightning/gelf-forwarder \
		--go_out=. --go-vtproto_out=. --go-grpc_out=. event.proto

.PHONY: default clean proto
