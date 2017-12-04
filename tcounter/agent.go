package tcounter

import (
    "net"
    "os"
    "time"
    "encoding/binary"
    "bytes"
    "log"
    "sync"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
    _ "net/http/pprof"
    "net/http"
)

type payload_send_value struct {
    key   counter_key
    value counter_value
}

type CounterAgent struct {
    Sock string
    Addr string
    table counter_map
    tableLock sync.RWMutex
    quit chan interface{}
    reqs chan interface{}
}

func NewCounterAgent() (obj *CounterAgent) {
    obj = &CounterAgent{}
    obj.table = make(counter_map)
    obj.quit = make(chan interface{}, 10)
    obj.reqs = make(chan interface{}, 10)
    return obj
}

func (self *CounterAgent) Start() {
    go func() {
        http.ListenAndServe(":8102", nil)
    }()

    // listen on local unix sock
    os.Remove(self.Sock)
    conn, err := net.ListenPacket("unixgram",  self.Sock)
    if err != nil {
        panic(err)
    }
    defer os.Remove(self.Sock)

    // dial to server
    conn2, err := grpc.Dial(self.Addr, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("cannot connect to %s: %v", self.Addr, err)
    }
    clt := NewCounterServiceClient(conn2)

    go self.writeLoop(clt)
    go self.flushTableLoop()
    self.readLoop(conn)
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

func (self *CounterAgent) getTable() (ret counter_map) {
    self.tableLock.RLock()
    defer self.tableLock.RUnlock()
    return self.table
}

func (self *CounterAgent) setTable(table counter_map) (oldTable counter_map) {
    self.tableLock.Lock()
    defer self.tableLock.Unlock()
    oldTable = self.table
    self.table = table
    return oldTable
}

func (self *CounterAgent) parseSendValue(payload *bytes.Buffer) {
    decoded := &payload_send_value{}
    binary.Read(payload, binary.BigEndian, &decoded.key)
    binary.Read(payload, binary.BigEndian, &decoded.value)

    table := self.getTable()  // must be in one thread
    mapped, ok := table[decoded.key]
    if !ok {
        //log.Printf("not mapped")
        mapped = &counter_mapped{}
        table[decoded.key] = mapped
    }
    //log.Printf("key(%v)", decoded.key)
    lenList := len(mapped.valueList)
    now := time.Now().Unix() / alignment * alignment
    if lenList > 0 {
        lastElem := mapped.valueList[lenList - 1]
        if now == lastElem.time {
            lastElem.sum = lastElem.sum + decoded.value
            lastElem.count++
            //log.Printf("update element")
        } else if now > lastElem.time {
            // append new element
            elem := &value_tick{now, decoded.value, 1}
            mapped.valueList = append(mapped.valueList, elem)
            //log.Printf("append a new element")
        }
    } else {
        // init with a new element
        elem := &value_tick{now, decoded.value, 1}
        mapped.valueList = make([]*value_tick, 1, send_table_interval / alignment * 2)
        mapped.valueList[0] = elem
        //log.Printf("init with a new element")
    }
}

func (self *CounterAgent) readLoop(conn net.PacketConn) {
    var buf [0xffff]byte
    for {
        n, _, err := conn.ReadFrom(buf[:])
        if err != nil {
            log.Fatalf("%v", err)
        }
        self.parseCommand(buf[:n])

        //v := self.table[100]
        //log.Printf("\n\n=== dump:")
        //for _, e := range v.valueList {
        //    log.Printf("=== %v", e)
        //}
    }
}

func (self *CounterAgent) flushTable() {
    table := self.setTable(make(counter_map))
    reqTable := make(map[uint32]*CounterMapped)
    for k, v := range table {
        valueList := make([]*ValueTick, len(v.valueList))
        for i, e := range v.valueList {
            valueList[i] = &ValueTick{e.time, e.sum, e.count}
        }
        reqTable[k] = &CounterMapped{valueList}
    }

    req := &SendTableReq{reqTable}
    self.reqs <- req
}

func (self *CounterAgent) flushTableLoop() {
    for {
        select {
        case <-self.quit:
            return
        default:
        }
        time.Sleep(send_table_interval * 1e9)
        self.flushTable()
    }
}

func (self *CounterAgent) writeLoop(clt CounterServiceClient) {
    for {
        select {
        case <-self.quit:
            return
        case req_ := <-self.reqs:
            req := req_.(*SendTableReq)
            _, err := clt.SendTable(context.Background(), req)
            if err != nil {
                log.Fatalf("SendTable err: %v", err)
            }
        }
    }
}