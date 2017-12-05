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
    "encoding/json"
    "strconv"
    "errors"
    "sync"
    "math"
)

var (
    EMPTY_RSP EmptyRsp
)

type DbConfig struct {
    Host string
    Port int
    Name string
    User string
    Pass string
}

type HttpRsp struct {
    Ver  int
    Code int
    Msg  string
    Data interface{}
}

var EMPTY_HTTP_RSP_DATA struct{}

type ChartPoint struct {
    S int64
    C uint64
}

type HttpChartRspData struct {
    Id uint32
    Begin int64
    End int64
    Interval int64
    Data []ChartPoint
}

type CounterServer struct {
    Addr string
    DbCfg DbConfig
    HttpAddr string
    table counter_map
    tableLock sync.RWMutex
    quit chan interface{}
    sendTableReqs chan interface{}
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

    dbStr := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8", self.DbCfg.User, self.DbCfg.Pass, self.DbCfg.Host, self.DbCfg.Port, self.DbCfg.Name)
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

    go self.serveHttp()

    go self.handleSendTableReqLoop()

    svr := grpc.NewServer()
    RegisterCounterServiceServer(svr, self)
    // Register reflection service on gRPC server.
    reflection.Register(svr)
    if err := svr.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}

func (self *CounterServer) serveHttp() {
    sv := http.NewServeMux()
    sv.HandleFunc("/cgi/chart", self.handleHttpChart)
    err := http.ListenAndServe(self.HttpAddr, sv)
    if err != nil {
        log.Fatalf("%v", err)
    }
}

func (self *CounterServer) getHttpGetParam(r *http.Request, param string) (ret string, err error) {
    if v, ok := r.URL.Query()[param]; ok {
        return v[0], nil
    }
    return "", errors.New(fmt.Sprintf("param('%s') is missing", param))
}

func (self *CounterServer) handleHttpChart(w http.ResponseWriter, r *http.Request) {
    var (
        id counter_key
        begin, end, align int64
        callback string
    )

    // get id
    if v, err := self.getHttpGetParam(r, "id"); err != nil || v == "" {
        responseError(w, 1, 11, err.Error())
        return
    } else {
        v, err := strconv.ParseUint(v, 10, 32)
        if err != nil {
            responseError(w, 1, 12, err.Error())
            return
        }
        id = counter_key(v)
    }

    // get begin
    if v, err := self.getHttpGetParam(r, "begin"); err != nil || len(v) == 0 {
        tm := time.Now()
        tm = time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, tm.Location())
        begin = tm.Unix()
    } else {
        v, err := strconv.ParseInt(v, 10, 64)
        if err != nil {
            responseError(w, 1, 13, err.Error())
            return
        }
        begin = v
    }

    // get end
    if v, err := self.getHttpGetParam(r, "end"); err != nil || len(v) == 0 {
        end = time.Now().Unix() - cahce_left_offset
    } else {
        v, err := strconv.ParseInt(v, 10, 64)
        if err != nil {
            responseError(w, 1, 14, err.Error())
            return
        }
        end = v
    }

    if end < begin || begin < 0 || end < 0 {
        err := errors.New("begin or end value err")
        responseError(w, 1, 14, err.Error())
        return
    }

    // get align
    if v, err := self.getHttpGetParam(r, "align"); err != nil || len(v) == 0 {
        align = alignment
    } else {
        v, err := strconv.ParseInt(v, 10, 64)
        if err != nil {
            responseError(w, 1, 14, err.Error())
            return
        }
        align = v
    }
    if align < alignment {
        align = alignment
    }

    // get callback, for jsonp
    if v, err := self.getHttpGetParam(r, "callback"); err != nil || len(v) == 0 {
        callback = ""
    } else {
        callback = v
    }

    // rsp
    begin = begin / alignment * alignment
    end = end / alignment * alignment
    pts, err := self.loadTableMapped(id, begin, end)
    if err != nil {
        responseError(w, 1, 15, err.Error())
        return
    }

    if align > alignment {
        begin = begin / align * align
        newPts := make([]ChartPoint, 1, int64(float64(align) / alignment * float64(len(pts))) + 2)
        var j int64 = 0
        for i, pt := range pts {
            tm := begin + int64(i) * alignment
            newTm := tm / align * align
            curJ := (newTm - begin) / align

            if curJ > j {
                j = curJ
                newPts = append(newPts, ChartPoint{})
            }
            newPts[j].S += pt.S
            newPts[j].C += pt.C
        }
        end = begin + j * align
        pts = newPts
    }

    data := &HttpChartRspData{id, begin, end, align, pts}
    if callback != "" {
        responeJsonp(w, 1, data, callback)
    } else {
        responseData(w, 1, data)
    }
}

func (self *CounterServer) loadTableMapped(key counter_key, begin int64, end int64) (ret []ChartPoint, err error) {
    sql := fmt.Sprintf("SELECT `ts`, `sum`, `count` FROM `values_%d` WHERE `ts`>=? AND `ts`<=? ORDER BY `ts`;", key)
    row, err := self.db.Query(sql, begin, end)
    if err != nil {
        log.Printf("query db failed(%v): %v", sql, err)
        return nil, err
    }

    num := (end - begin) / alignment + 1
    ret = make([]ChartPoint, num)

    for row.Next() {
        var ts int64
        pt := ChartPoint{}
        row.Scan(&ts, &pt.S, &pt.C)
        i := (ts - begin) / alignment
        ret[i] = pt
    }
    row.Close()

    self.tableLock.RLock()
    if v, ok := self.table[key]; ok {
        base := v.valueList[v.saveBegin].time
        mergeBegin := int64(math.Floor(float64(begin - base) / alignment)) + v.saveBegin  // must use floor div
        mergeEnd := int64(math.Floor(float64(end - base) / alignment)) + v.saveBegin
        //log.Printf("http %v->%v, mem %v->%v", mergeBegin, mergeEnd, v.saveBegin, v.saveEnd)
        //log.Printf("@1@%v, %v, %v, %v, %v, %v, %v, %v", begin, end, v.saveBegin, v.saveEnd, mergeBegin, mergeEnd, base, num)
        if v.saveBegin <= mergeEnd && v.saveEnd >= mergeBegin {
            // merge
            if mergeBegin < v.saveBegin {
                mergeBegin = v.saveBegin
            }
            if mergeEnd > v.saveEnd {
                mergeEnd = v.saveEnd
            }
            //log.Printf("@2@%v, %v, %v, %v, %v, %v, %v", begin, end, v.saveBegin, v.saveEnd, mergeBegin, mergeEnd, base)
            for i:=mergeBegin; i<=mergeEnd; i++ {
                value := v.valueList[i]
                j := (value.time - begin) / alignment
                //log.Printf("@3@%v, %v, %v, %v, %v, %v, %v, %v", begin, end, v.saveBegin, v.saveEnd, mergeBegin, mergeEnd, value.time, j)
                ret[j].S += value.sum
                ret[j].C += value.count
            }
        }
    }
    self.tableLock.RUnlock()

    return ret, nil
}

func responseError(w http.ResponseWriter, ver int, errcode int, errmsg string) {
    w.Header().Add("content-type", "application/json; charset=utf-8")
    rsp := HttpRsp{
        ver,
        errcode,
        errmsg,
        EMPTY_HTTP_RSP_DATA,
    }
    jsStr, err := json.Marshal(rsp)
    if err != nil {
        log.Printf(err.Error())
        return
    }
    w.Write(jsStr)
}

func responseData(w http.ResponseWriter, ver int, data interface{}) {
    w.Header().Add("content-type", "application/json; charset=utf-8")
    rsp := HttpRsp{
        ver,
        0,
        "",
        data,
    }
    jsStr, err := json.Marshal(rsp)
    if err != nil {
        rsp := HttpRsp{
            ver,
            -1,
            err.Error(),
            EMPTY_HTTP_RSP_DATA,
        }
        jsStr, err = json.Marshal(rsp)
        if err != nil {
            log.Printf(err.Error())
            return
        }
    }
    w.Write(jsStr)
}

func responeJsonp(w http.ResponseWriter, ver int, data interface{}, callback string) {
    w.Header().Add("content-type", "application/json; charset=utf-8")
    rsp := HttpRsp{
        ver,
        0,
        "",
        data,
    }
    jsStr, err := json.Marshal(rsp)
    if err != nil {
        rsp := HttpRsp{
            ver,
            -1,
            err.Error(),
            EMPTY_HTTP_RSP_DATA,
        }
        jsStr, err = json.Marshal(rsp)
        if err != nil {
            log.Printf(err.Error())
            return
        }
    }
    w.Write([]byte(fmt.Sprintf("%s(%s);", callback, jsStr)))
}

func (self *CounterServer) SendTable(ctx context.Context, req *SendTableReq) (rsp *EmptyRsp, err error) {
    self.sendTableReqs <- req
    return &EMPTY_RSP, nil
}

func (self *CounterServer) saveTableMapped(key counter_key, mapped *counter_mapped) {
    // TODO: save table mapped of key to db
    log.Printf("\n\n=== save key(%v) mapped (%v -> %v)", key, mapped.valueList[mapped.saveBegin].time, mapped.valueList[mapped.saveEnd].time)

    num := mapped.saveEnd - mapped.saveBegin + 1
    valueStrings := make([]string, 0, num)
    valueArgs := make([]interface{}, 0, num * 3)
    for i:=mapped.saveBegin; i<=mapped.saveEnd; i++ {
        value := mapped.valueList[i]
        log.Printf("=== %v", *value)
        if value.count <= 0 {
            continue
        }
        valueStrings = append(valueStrings, "(?, ?, ?)")
        valueArgs = append(valueArgs, value.time)
        valueArgs = append(valueArgs, value.sum)
        valueArgs = append(valueArgs, value.count)
    }
    if len(valueStrings) == 0 {
        return
    }
    sql := fmt.Sprintf("INSERT INTO `values_%d` (`ts`, `sum`, `count`) VALUES %s ON DUPLICATE KEY UPDATE `sum`=VALUES(`sum`), `count`=VALUES(`count`);", key, strings.Join(valueStrings, ", "))
    row, err := self.db.Query(sql, valueArgs...)
    if err != nil {
        log.Printf("query db failed(%v): %v", sql, err)
        return
    }
    row.Close()
}

func (self *CounterServer) handleSendTableReq(req *SendTableReq) {
    self.tableLock.Lock()
    defer self.tableLock.Unlock()
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
            mapped.valueList = make([]*value_tick, cache_size)  // [now - 90, now + 90)

            now := time.Now().Unix() / alignment * alignment
            from := now - cahce_left_offset

            saveBegin := (reqTimeBegin - from) / alignment
            if saveBegin >= 0 {
                mapped.saveBegin = saveBegin
            } else {
                mapped.saveBegin = 0
            }

            saveEnd := (reqTimeEnd - from) / alignment
            if saveEnd < cache_size {
                if saveEnd >= 0 {
                    mapped.saveEnd = saveEnd
                } else {
                    mapped.saveEnd = 0
                }
            } else {
                mapped.saveEnd = cache_size - 1
            }

            j, n := 0, len(v.ValueList)
            for i:=0; i<cache_size; i++ {
                tm := from + int64(i*alignment)
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
                mapped.valueList = make([]*value_tick, cache_size)  // [now - 90, now + 90)

                now := time.Now().Unix() / alignment * alignment
                from := now - cahce_left_offset

                saveBegin := (reqTimeBegin - from) / alignment
                if saveBegin >= 0 {
                    mapped.saveBegin = saveBegin
                } else {
                    mapped.saveBegin = 0
                }

                saveEnd := (reqTimeEnd - from) / alignment
                if saveEnd < cache_size {
                    if saveEnd >= 0 {
                        mapped.saveEnd = saveEnd
                    } else {
                        mapped.saveEnd = 0
                    }
                } else {
                    mapped.saveEnd = cache_size - 1
                }

                j, n := 0, len(v.ValueList)
                for i:=0; i<cache_size; i++ {
                    tm := from + int64(i*alignment)

                    if k := (tm - base) / alignment; k < cache_size {
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
                saveBegin := (reqTimeBegin - base) / alignment
                if saveBegin < mapped.saveBegin {
                    if saveBegin > 0 {
                        mapped.saveBegin = saveBegin
                    } else {
                        mapped.saveBegin = 0
                    }
                }
                saveEnd := (reqTimeEnd - base) / alignment
                if saveEnd > mapped.saveEnd {
                    mapped.saveEnd = saveEnd
                }
                for _, e := range v.ValueList {
                    i := (e.Time - base) / alignment
                    if i < 0 || i >= cache_size {
                        continue
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