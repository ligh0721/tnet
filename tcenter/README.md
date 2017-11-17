[Quick Start](https://grpc.io/docs/quickstart/go.html)
```
go get -u github.com/golang/protobuf/protoc-gen-go
go get -u google.golang.org/grpc

protoc --go_out=plugins=grpc:. *.proto
```