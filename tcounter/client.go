package tcounter

import (
    "net"
    "log"
    "bytes"
    "encoding/binary"
    "io/ioutil"
    "os"
)

type CounterClient struct {
    Sock string
    conn net.PacketConn
    addr net.Addr
}

func NewCounterClient() (obj *CounterClient) {
    obj = &CounterClient{}
    return obj
}

func (self *CounterClient) Dial() (ret error) {
    f, err := ioutil.TempFile("", "tcounterc")
    if err != nil {
        return err
    }
    addr := f.Name()
    os.Remove(addr)

    self.conn, err = net.ListenPacket("unixgram", addr)
    if err != nil {
        log.Printf("%v", err)
        return err
    }
    defer os.Remove(addr)

    self.addr, err = net.ResolveUnixAddr("unixgram", self.Sock)
    if err != nil {
        log.Printf("%v", err)
        return err
    }

    return nil
}

func (self *CounterClient) Close() {
    self.conn.Close()
}

func (self *CounterClient) SendValue(key counter_key, value counter_value) {
    if self.conn == nil {
        return
    }
    buf := &bytes.Buffer{}
    decoded := &payload_send_value{key, value}
    binary.Write(buf, binary.BigEndian, cmd_send_value)
    binary.Write(buf, binary.BigEndian, decoded.key)
    binary.Write(buf, binary.BigEndian, decoded.value)
    encoded := buf.Bytes()
    self.conn.WriteTo(encoded, self.addr)
}