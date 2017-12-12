package tcenter

//go:generate protoc --go_out=plugins=grpc:. tcenter.proto

import (
    "net"
    "log"
    "google.golang.org/grpc/reflection"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
    "sync/atomic"
    "sync"
    "time"
    "fmt"
    "errors"
)

var (
    EMPTY_RSP = &EmptyRsp{}
)

type TCenterClientInfo struct {
    hostInfo *HostInfo
    lastHealth time.Time
}

type TCenterServer struct {
    Addr string
    lis net.Listener
    svr *grpc.Server
    idgen uint32
    clts *sync.Map
}

func NewTCenterServer() (obj *TCenterServer) {
    obj = &TCenterServer{}
    obj.idgen = 1000
    obj.clts = &sync.Map{}
    return obj
}

func (self *TCenterServer) Start() {
    var err error
    self.lis, err = net.Listen("tcp", self.Addr)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    self.svr = grpc.NewServer()
    RegisterTCenterServiceServer(self.svr, self)
    // Register reflection service on gRPC server.
    reflection.Register(self.svr)
    if err := self.svr.Serve(self.lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}

func (self *TCenterServer) nextId() (ret uint32) {
    return atomic.AddUint32(&self.idgen, 1)
}

func printClientInfo(id uint32, info *TCenterClientInfo) {
    s := fmt.Sprintf("os: %s\narch: %s\nhostname: %s", info.hostInfo.Os, info.hostInfo.Arch)
    if info.hostInfo.Interfaces != nil {
        s = s + fmt.Sprintf("\ninterfaces(%d):\n", len(info.hostInfo.Interfaces))
        for _, itf := range info.hostInfo.Interfaces {
            s = s + fmt.Sprintf("  name: %s\n    mac: %s\n    ip: %s\n    mask: %s\n", itf.Name, itf.Mac, itf.Ip, itf.Mask)
        }
    }
    s = s + fmt.Sprintf("envs(%d):\n", len(info.hostInfo.Envs))
    for _, env := range info.hostInfo.Envs {
        s = s + fmt.Sprintf("    %s\n", env)
    }
    s = s + fmt.Sprintf("numcpu: %d\n", info.hostInfo.Numcpu)

    log.Printf("client(%d) info:\n%s", id, s)
}

func (self *TCenterServer) storeClientInfo(id uint32, info *TCenterClientInfo) {
    now := time.Now()
    var todel []uint32
    self.clts.Range(func(key, value interface{}) bool {
        k := key.(uint32)
        v := value.(*TCenterClientInfo)
        delta := now.Unix() - v.lastHealth.Unix()
        if delta > 120e9 {
            log.Printf("@@ %v delta %v", k, delta)
            todel = append(todel, k)
        }
        return true
    })
    for _, d := range todel {
        self.clts.Delete(d)
    }
    self.clts.Store(id, info)
}

func (self *TCenterServer) Login(ctx context.Context, req *LoginReq) (rsp *LoginRsp, err error) {
    id := self.nextId()
    info := &TCenterClientInfo{}
    info.hostInfo = req.HostInfo
    info.lastHealth = time.Now()
    self.clts.Store(id, info)
    //self.storeClientInfo(id, info)
    log.Printf("client(%d) login", id)
    printClientInfo(id, info)

    rsp = &LoginRsp{}
    rsp.Id = id
    return rsp, nil
}

func (self *TCenterServer) Health(ctx context.Context, req *HealthReq) (rsp *EmptyRsp, err error) {
    id := req.Id
    v, ok := self.clts.Load(id)
    if !ok {
        err = errors.New(fmt.Sprintf("invalid client(%d)", id))
        log.Printf("%v", err)
        return nil, err
    }

    value := v.(*TCenterClientInfo)
    if req.HostInfo != nil {
        value.hostInfo = req.HostInfo
        //self.storeClientInfo(req.Id, value)
    }
    value.lastHealth = time.Now()
    return EMPTY_RSP, nil
}

func (self *TCenterServer) ListClients(ctx context.Context, req *ListClientsReq) (rsp *ListClientsRsp, err error) {
    id := req.Id
    _, ok := self.clts.Load(id)
    if !ok {
        err = errors.New(fmt.Sprintf("invalid client(%d)", id))
        log.Printf("%v", err)
        return nil, err
    }

    rsp = &ListClientsRsp{}

    var todel []uint32
    now := time.Now().Unix()
    self.clts.Range(func(key, value interface{}) bool {
        k := key.(uint32)
        v := value.(*TCenterClientInfo)

        delta := now - v.lastHealth.Unix()
        if delta > 120e9 {
            log.Printf("@@ %v delta %v", k, delta)
            todel = append(todel, k)
            return true
        }

        info := &ListClientsRsp_ClientInfo{}
        info.Id = k
        info.HostInfo = v.hostInfo
        info.LastHealth = v.lastHealth.Unix()
        rsp.ClientInfos = append(rsp.ClientInfos, info)
        return true
    })

    for _, d := range todel {
        log.Printf("@del %v", d)
        self.clts.Delete(d)
    }

    return rsp, nil
}