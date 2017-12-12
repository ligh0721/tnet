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
    conn net.PacketConn
    addr net.Addr
}

func NewCounterClientUseUnix(sock string) (obj *CounterClient) {
    obj = &CounterClient{}

    f, err := ioutil.TempFile("", "tcounterc")
    if err != nil {
        return nil
    }
    addr := f.Name()
    os.Remove(addr)

    obj.conn, err = net.ListenPacket("unixgram", addr)
    if err != nil {
        log.Printf("%v", err)
        return nil
    }
    defer os.Remove(addr)

    obj.addr, err = net.ResolveUnixAddr("unixgram", sock)
    if err != nil {
        log.Printf("%v", err)
        return nil
    }

    return obj
}

func NewCounterClientUseUdp(raddr string) (obj *CounterClient) {
    obj = &CounterClient{}

    var err error
    obj.conn, err = net.ListenPacket("udp", ":")
    if err != nil {
        log.Printf("%v", err)
        return nil
    }

    obj.addr, err = net.ResolveUDPAddr("udp", raddr)
    if err != nil {
        log.Printf("%v", err)
        return nil
    }

    return obj
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