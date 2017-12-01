package tcounter

//go:generate protoc --go_out=plugins=grpc:. tcounter.proto

const (
    cmd_send_value uint32 = 0

    type_add = 0
    type_set = 1

    send_table_interval = 10  // flush table per 10 seconds
    cahce_left_offset = send_table_interval * 2
    cache_range = cahce_left_offset + send_table_interval * 1

)

type counter_key = uint32
type counter_value = int64

type value_tick struct {
    time int64
    sum counter_value
    count uint64
}

type counter_mapped struct {
    valueList []*value_tick
    saveBegin int64
    saveEnd int64
}

type counter_map = map[counter_key]*counter_mapped
