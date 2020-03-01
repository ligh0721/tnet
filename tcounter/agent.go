package tcounter

import (
	"bytes"
	"encoding/binary"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"
)

type payload_send_value struct {
	key   counter_key
	value counter_value
}

type CounterAgent struct {
	conn      net.PacketConn
	raddr     string
	table     counter_map
	tableLock sync.Mutex
	quit      chan interface{}
	reqs      chan interface{}
}

func NewCounterAgentUseUnix(sock string, serverAddr string) (obj *CounterAgent) {
	obj = &CounterAgent{}
	obj.table = make(counter_map)
	obj.quit = make(chan interface{}, 10)
	obj.reqs = make(chan interface{}, 10)

	os.Remove(sock)
	var err error
	obj.conn, err = net.ListenPacket("unixgram", sock)
	if err != nil {
		log.Fatalf("%v", err)
	}
	obj.raddr = serverAddr
	return obj
}

func NewCounterAgentUseUdp(laddr string, serverAddr string) (obj *CounterAgent) {
	obj = &CounterAgent{}
	obj.table = make(counter_map)
	obj.quit = make(chan interface{}, 10)
	obj.reqs = make(chan interface{}, 10)

	var err error
	obj.conn, err = net.ListenPacket("udp", laddr)
	if err != nil {
		log.Fatalf("%v", err)
	}
	obj.raddr = serverAddr
	return obj
}

func (self *CounterAgent) Start() {
	defer os.Remove(self.conn.LocalAddr().String())
	go func() {
		http.ListenAndServe(":8102", nil)
	}()

	// dial to server
	rconn, err := grpc.Dial(self.raddr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("cannot connect to %s: %v", self.raddr, err)
	}
	clt := NewCounterServiceClient(rconn)

	go self.writeLoop(clt)
	go self.flushTableLoop()
	self.readLoop()
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

	self.tableLock.Lock()
	defer self.tableLock.Unlock()
	table := self.table
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
		lastElem := mapped.valueList[lenList-1]
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
		mapped.valueList = make([]*value_tick, 1, send_table_interval/alignment*2)
		mapped.valueList[0] = elem
		//log.Printf("init with a new element")
	}
}

func (self *CounterAgent) readLoop() {
	var buf [0xffff]byte
	for {
		n, _, err := self.conn.ReadFrom(buf[:])
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
	self.tableLock.Lock()
	table := self.table
	self.table = make(counter_map)
	self.tableLock.Unlock()
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
