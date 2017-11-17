package tcenter

import (
    "log"
    "os"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
    "runtime"
    "net"
    "time"
)

type TCenterClient struct {
    Addr string
}

func NewTCenterClient() (obj *TCenterClient) {
    obj = &TCenterClient{}
    return obj
}

func (self *TCenterClient) Start() {
    // Set up a connection to the server.
    conn, err := grpc.Dial(self.Addr, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("did not connect: %v", err)
    }
    defer conn.Close()
    c := NewTCenterServiceClient(conn)

    loginReq := &LoginReq{}
    loginReq.HostInfo = &HostInfo{}
    lastInfoStr := getHostInfo(loginReq.HostInfo)

    rsp, err := c.Login(context.Background(), loginReq)
    if err != nil {
        log.Fatalf("could not rpc login: %v", err)
        //return
    }
    id := rsp.Id
    log.Printf("rsp: client(%d)", id)

    for {
        healthReq := &HealthReq{}
        healthReq.Id = id
        hostInfo := &HostInfo{}
        infoStr := getHostInfo(hostInfo)
        if infoStr != lastInfoStr {
            healthReq.HostInfo = hostInfo
            log.Printf("host info updated")
        }
        _, err := c.Health(context.Background(), healthReq)
        if err != nil {
            log.Fatalf("could not rpc health: %v", err)
            //return
        }
        time.Sleep(60e9)
    }
}

func getHostInfo(info *HostInfo) (ret string) {
    info.Os = runtime.GOOS
    info.Arch = runtime.GOARCH
    info.Hostname, _ = os.Hostname()
    itfs, _ := net.Interfaces()
    info.Interfaces = make([]*IfInfo, len(itfs))
    for i, itf := range itfs {
        ifInfo := &IfInfo{}
        info.Interfaces[i] = ifInfo
        ifInfo.Name = itf.Name
        ifInfo.Mac = itf.HardwareAddr.String()
        addrs, _ := itf.Addrs()
        for _, addr := range addrs {
            if ipnet, ok := addr.(*net.IPNet); ok {
                if ipnet.IP.To4() != nil {
                    ifInfo.Ip = ipnet.IP.String()
                    ifInfo.Mask = net.IP(ipnet.Mask).String()
                    break
                }
            }
        }
    }
    info.Envs = os.Environ()
    info.Numcpu = int32(runtime.NumCPU())
    return info.String()
}