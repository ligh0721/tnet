package tcounter

import (
    "net"
    "log"
    "google.golang.org/grpc/reflection"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
    "time"
)

var (
    EMPTY_RSP = &EmptyRsp{}
)

type CounterServer struct {
    Addr string
    table counter_map
    quit chan interface{}
    sendTableReqs chan interface{}
}

func NewCounterServer() (obj *CounterServer) {
    obj = &CounterServer{}
    obj.table = make(counter_map)
    obj.quit = make(chan interface{}, 10)
    obj.sendTableReqs = make(chan interface{}, 128)
    return obj
}

func (self *CounterServer) Start() {
    lis, err := net.Listen("tcp", self.Addr)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    defer lis.Close()

    go self.handleSendTableReqLoop()

    svr := grpc.NewServer()
    RegisterCounterServiceServer(svr, self)
    // Register reflection service on gRPC server.
    reflection.Register(svr)
    if err := svr.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}

func (self *CounterServer) SendTable(ctx context.Context, req *SendTableReq) (rsp *EmptyRsp, err error) {
    self.sendTableReqs <- req
    return EMPTY_RSP, nil
}

func (self *CounterServer) saveTableMapped(key counter_key, mapped *counter_mapped) {
    // TODO: save table mapped of key to db
    log.Printf("\n\n=== save key(%v) mapped (%v -> %v)", key, mapped.valueList[0].time, mapped.valueList[len(mapped.valueList) - 1].time)
    for _, e := range mapped.valueList {
        log.Printf("=== %v", *e)
    }
}

func (self *CounterServer) handleSendTableReq(req *SendTableReq) {
    for k, v := range req.Table {
        //var mapped *counter_mapped_sync
        mapped, ok := self.table[k]
        if !ok {
            // key isnot exist, init new value list
            log.Printf("key isnot exist, init new value list")
            mapped = &counter_mapped{}
            self.table[k] = mapped
            mapped.valueList = make([]*value_tick, cache_range)  // [now - 90, now + 90)

            now := time.Now().Unix()
            from := now - cahce_left_offset
            j, n := 0, len(v.ValueList)
            for i:=0; i<cache_range; i++ {
                tm := from + int64(i)
                if j < n && tm == v.ValueList[j].Time {
                    mapped.valueList[i] = &value_tick{tm, v.ValueList[j].Sum, v.ValueList[j].Count}
                    j++
                } else {
                    mapped.valueList[i] = &value_tick{tm, 0, 0}
                }
            }
        } else {
            base := mapped.valueList[0].time
            if v.ValueList[len(v.ValueList) - 1].Time - base >= cache_range {
                // not enough, init new value list
                log.Printf("not enough, init new value list")
                self.saveTableMapped(k, mapped)

                oldmapped := mapped
                mapped = &counter_mapped{}
                self.table[k] = mapped
                mapped.valueList = make([]*value_tick, cache_range)  // [now - 90, now + 90)

                now := time.Now().Unix()
                from := now - cahce_left_offset
                j, n := 0, len(v.ValueList)
                for i:=0; i<cache_range; i++ {
                    tm := from + int64(i)

                    if k := tm - base; k < cache_range {
                        // use oldmapped data
                        mapped.valueList[i] = oldmapped.valueList[k]
                    } else {
                        mapped.valueList[i] = &value_tick{tm, 0, 0}
                    }

                    if j < n && tm == v.ValueList[j].Time {
                        mapped.valueList[i].sum += v.ValueList[j].Sum
                        mapped.valueList[i].count += v.ValueList[j].Count
                        j++
                    }
                }
            } else {
                // merge values
                log.Printf("merge values")
                for _, e := range v.ValueList {
                    i := e.Time - base
                    if i < 0 || i >= cache_range {
                        break
                    }
                    mapped.valueList[i].sum += e.Sum
                    mapped.valueList[i].count += e.Count
                }
            }
        }
    }
}

func (self *CounterServer) handleSendTableReqLoop() {
    for {
        select {
        case <-self.quit:
            return
        case req_ := <-self.sendTableReqs:
            req := req_.(*SendTableReq)
            self.handleSendTableReq(req)
        }
    }
}