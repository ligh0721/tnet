package tcenter

import (
    "log"
    "os"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
    "runtime"
    "net"
    "time"
    "fmt"
)

type TCenterClient struct {
    Addr string
    Id uint32
    conn *grpc.ClientConn
    clt TCenterServiceClient
}

func NewTCenterClient() (obj *TCenterClient) {
    obj = &TCenterClient{}
    return obj
}

func (self *TCenterClient) Start() {
    // Set up a connection to the server.
    var err error
    self.conn, err = grpc.Dial(self.Addr, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("did not connect: %v", err)
    }
    self.clt = NewTCenterServiceClient(self.conn)
}

func (self *TCenterClient) Close() {
    self.conn.Close()
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

func (self *TCenterClient) Login() (ret string) {
    loginReq := &LoginReq{}
    loginReq.HostInfo = &HostInfo{}
    ret = getHostInfo(loginReq.HostInfo)

    rsp, err := self.clt.Login(context.Background(), loginReq)
    if err != nil {
        log.Fatalf("could not rpc login: %v", err)
        //return
    }
    self.Id = rsp.Id
    log.Printf("rsp: client(%d)", self.Id)
    return ret
}

func (self *TCenterClient) HealthLoop(loginInfoStr string) {
    for {
        healthReq := &HealthReq{}
        healthReq.Id = self.Id
        hostInfo := &HostInfo{}
        infoStr := getHostInfo(hostInfo)
        if infoStr != loginInfoStr {
            healthReq.HostInfo = hostInfo
            log.Printf("host info updated")
        }
        _, err := self.clt.Health(context.Background(), healthReq)
        if err != nil {
            log.Fatalf("could not rpc health: %v", err)
            //return
        }
        time.Sleep(60e9)
    }
}

func getClientInfoStr(info *HostInfo) (ret string) {
    s := fmt.Sprintf("os: %s\narch: %s\nhostname: %s", info.Os, info.Arch, info.Hostname)
    if info.Interfaces != nil {
        s = s + fmt.Sprintf("\ninterfaces(%d):\n", len(info.Interfaces))
        for _, itf := range info.Interfaces {
            s = s + fmt.Sprintf("  name: %s\n    mac: %s\n    ip: %s\n    mask: %s\n", itf.Name, itf.Mac, itf.Ip, itf.Mask)
        }
    }
    s = s + fmt.Sprintf("envs(%d):\n", len(info.Envs))
    for _, env := range info.Envs {
        s = s + fmt.Sprintf("    %s\n", env)
    }
    s = s + fmt.Sprintf("numcpu: %d\n", info.Numcpu)
    return s
}

func (self *TCenterClient) ListClients() {
    listClientsReq := &ListClientsReq{}
    listClientsReq.Id = self.Id
    rsp, err := self.clt.ListClients(context.Background(), listClientsReq)
    if err != nil {
        log.Fatalf("could not rpc list clients: %v", err)
        //return
    }
    s := ""
    for _, info := range rsp.GetClientInfos() {
        s = s + fmt.Sprintf("--------------------\nID: %d\nLAST: %s\n%s", info.Id, time.Unix(info.LastHealth, 0).Format("2006-01-02 15:04:05"), getClientInfoStr(info.HostInfo))
    }
    log.Printf("total %d client(s) info:\n%s", len(rsp.GetClientInfos()), s)
}