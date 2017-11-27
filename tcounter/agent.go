package tcounter

import (
    "net"
    "os"
    "time"
    "encoding/binary"
    "bytes"
    "log"
)



type time_count struct {
    time int64
    sum counter_value
    count uint64
}

type counter_mapped struct {
    list []*time_count
}

type counter_map = map[counter_key]*counter_mapped

type payload_send_value struct {
    key   counter_key
    value counter_value
}

type CounterAgent struct {
    Sock string
    Map counter_map
}

func NewCounterAgent() (obj *CounterAgent) {
    obj = &CounterAgent{}
    obj.Map = make(counter_map)
    return obj
}

func (self *CounterAgent) parseCommand(data []byte) {
    payload := bytes.NewBuffer(data)
    var cmd uint32
    binary.Read(payload, binary.BigEndian, &cmd)
    switch cmd {
    case cmd_send_value:
        self.parseSendValue(payload)
        break
    default:
        log.Fatal("unknown cmd(%v)", cmd)
    }
}

func (self *CounterAgent) parseSendValue(payload *bytes.Buffer) {
    decoded := &payload_send_value{}
    binary.Read(payload, binary.BigEndian, &decoded.key)
    binary.Read(payload, binary.BigEndian, &decoded.value)

    mapped, ok := self.Map[decoded.key]
    if !ok {
        //log.Printf("not mapped")
        mapped = &counter_mapped{}
        self.Map[decoded.key] = mapped
    }
    //log.Printf("key(%v)", decoded.key)
    lenList := len(mapped.list)
    now := time.Now().Unix()
    if lenList > 0 {
        lastElem := mapped.list[lenList - 1]
        if now == lastElem.time {
            lastElem.sum = lastElem.sum + decoded.value
            lastElem.count++
            //log.Printf("update element")
        } else if now > lastElem.time {
            // append new element
            elem := &time_count{now, decoded.value, 1}
            mapped.list = append(mapped.list, elem)
            //log.Printf("append a new element")
        }
    } else {
        // init with a new element
        elem := &time_count{now, decoded.value, 1}
        mapped.list = make([]*time_count, 60)[:1]
        mapped.list[0] = elem
        //log.Printf("init with a new element")
    }
}

func (self *CounterAgent) Start() {
    conn, err := net.ListenUnixgram("unixgram",  &net.UnixAddr{self.Sock, "unixgram"})
    if err != nil {
        panic(err)
    }
    defer os.Remove(self.Sock)

    var buf [0xffff]byte
    for {
        n, err := conn.Read(buf[:])
        if err != nil {
            log.Fatalf("%v", err)
        }
        self.parseCommand(buf[:n])

        //v := self.Map[100]
        //log.Printf("dump:")
        //for _, e := range v.list {
        //    log.Printf("%v", e)
        //}
    }
}

func (self *CounterAgent) archive() {

}