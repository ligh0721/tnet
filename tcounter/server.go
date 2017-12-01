package tcounter

import (
    "net"
    "log"
    "google.golang.org/grpc/reflection"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
    "time"
    "net/http"
    _ "net/http/pprof"
    "database/sql"
    _ "github.com/go-sql-driver/mysql"
    "fmt"
    "strings"
)

var (
    EMPTY_RSP = &EmptyRsp{}
)

type DbConfig struct {
    Host string
    Port int
    Name string
    User string
    Pass string
}

type CounterServer struct {
    Addr string
    table counter_map
    quit chan interface{}
    sendTableReqs chan interface{}
    dbCfg DbConfig
    db *sql.DB
}

func NewCounterServer() (obj *CounterServer) {
    obj = &CounterServer{}
    obj.table = make(counter_map)
    obj.quit = make(chan interface{}, 10)
    obj.sendTableReqs = make(chan interface{}, 128)
    return obj
}

func (self *CounterServer) Start() {
    go func() {
        http.ListenAndServe(":8101", nil)
    }()

    dbStr := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8", self.dbCfg.User, self.dbCfg.Pass, self.dbCfg.Host, self.dbCfg.Port, self.dbCfg.Name)
    var err error
    self.db, err = sql.Open("mysql", dbStr)
    if err != nil {
        log.Fatalf("connect db failed: %v", err)
    }

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

    num := mapped.saveEnd - mapped.saveBegin + 1
    valueStrings := make([]string, 0, num)
    valueArgs := make([]interface{}, 0, num * 3)
    for i:=mapped.saveBegin; i<=mapped.saveEnd; i++ {
        value := mapped.valueList[i]
        if value.count <= 0 {
            continue
        }
        log.Printf("=== %v", *value)
        valueStrings = append(valueStrings, "(?, ?, ?)")
        valueArgs = append(valueArgs, value.time)
        valueArgs = append(valueArgs, value.sum)
        valueArgs = append(valueArgs, value.count)
    }
    sql := fmt.Sprintf("INSERT INTO `values_%s` (`ts`, `sum`, `count`) VALUES %s ON DUPLICATE KEY UPDATE `sum`=VALUES(`sum`), `count`=VALUES(`count`);", key, strings.Join(valueStrings, ", "))
    stmt, err := self.db.Prepare(sql)
    if err != nil {
        log.Printf("prepare statement failed: %v", err)
        return
    }

    _, err = stmt.Exec(valueArgs...)
    stmt.Close()
    if err != nil {
        log.Printf("exec statement failed: %v", err)
        return
    }
}

func (self *CounterServer) handleSendTableReq(req *SendTableReq) {
    for k, v := range req.Table {
        //var mapped *counter_mapped_sync
        reqTimeBegin := v.ValueList[0].Time
        reqTimeEnd := v.ValueList[len(v.ValueList) - 1].Time
        mapped, ok := self.table[k]
        if !ok {
            // key isnot exist, init new value list
            log.Printf("key isnot exist, init new value list")

            mapped = &counter_mapped{}
            self.table[k] = mapped
            mapped.valueList = make([]*value_tick, cache_range)  // [now - 90, now + 90)

            now := time.Now().Unix()
            from := now - cahce_left_offset

            saveBegin := reqTimeBegin - from
            if saveBegin >= 0 {
                mapped.saveBegin = saveBegin
            } else {
                mapped.saveBegin = 0
            }

            saveEnd := reqTimeEnd - from
            if saveEnd < cache_range {
                if saveEnd >= 0 {
                    mapped.saveEnd = saveEnd
                } else {
                    mapped.saveEnd = 0
                }
            } else {
                mapped.saveEnd = cache_range - 1
            }

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
            if reqTimeEnd - base >= cache_range {
                // not enough, init new value list
                log.Printf("not enough, init new value list")

                self.saveTableMapped(k, mapped)

                oldmapped := mapped
                mapped = &counter_mapped{}
                self.table[k] = mapped
                mapped.valueList = make([]*value_tick, cache_range)  // [now - 90, now + 90)

                now := time.Now().Unix()
                from := now - cahce_left_offset

                saveBegin := reqTimeBegin - from
                if saveBegin >= 0 {
                    mapped.saveBegin = saveBegin
                } else {
                    mapped.saveBegin = 0
                }

                saveEnd := reqTimeEnd - from
                if saveEnd < cache_range {
                    if saveEnd >= 0 {
                        mapped.saveEnd = saveEnd
                    } else {
                        mapped.saveEnd = 0
                    }
                } else {
                    mapped.saveEnd = cache_range - 1
                }

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
                saveBegin := reqTimeBegin - base
                if saveBegin < mapped.saveBegin {
                    if saveBegin > 0 {
                        mapped.saveBegin = saveBegin
                    } else {
                        mapped.saveBegin = 0
                    }
                }
                saveEnd := reqTimeEnd - base
                if saveEnd > mapped.saveEnd {
                    mapped.saveEnd = saveEnd
                }
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