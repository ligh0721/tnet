package tcounter

import (
    "net"
    "log"
    "bytes"
    "encoding/binary"
)

type CounterClient struct {
    Sock string
    conn *net.UnixConn
}

func NewCounterClient() (obj *CounterClient) {
    obj = &CounterClient{}
    return obj
}

func (self *CounterClient) Dial() (ret error) {
    var err error
    self.conn, err = net.DialUnix("unixgram", nil, &net.UnixAddr{self.Sock, "unixgram"})
    if err != nil {
        log.Fatalf("%v", err)
        return err
    }

    return nil
}

func (self *CounterClient) Close() {
    self.conn.Close()
}

func (self *CounterClient) SendValue(key counter_key, value counter_value) {
    buf := bytes.NewBuffer([]byte{})
    decoded := &payload_send_value{key, value}
    binary.Write(buf, binary.BigEndian, cmd_send_value)
    binary.Write(buf, binary.BigEndian, decoded.key)
    binary.Write(buf, binary.BigEndian, decoded.value)
    encoded := buf.Bytes()
    self.conn.Write(encoded)
}