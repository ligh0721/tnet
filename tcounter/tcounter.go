package tcounter

//go:generate protoc --go_out=plugins=grpc:. tcounter.proto

const (
    cmd_send_value uint32 = 0

    type_add = 0
    type_set = 1
)

type counter_key = uint32
type counter_value = int64