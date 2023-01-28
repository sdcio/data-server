mkdir -p schema_server
protoc --go_out=schema_server --go-grpc_out=schema_server -I . schema.proto data.proto

