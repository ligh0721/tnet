package tcenter

import (
    "net"
    "log"
    "google.golang.org/grpc/reflection"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
)

type TCenterServer struct {
    Addr string
}

func NewTCenterServer() (obj *TCenterServer) {
    obj = &TCenterServer{}
    return obj
}

func (self *TCenterServer) Start() {
    lis, err := net.Listen("tcp", self.Addr)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    s := grpc.NewServer()
    RegisterTCenterServiceServer(s, self)
    // Register reflection service on gRPC server.
    reflection.Register(s)
    if err := s.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}

func (self *TCenterServer) Hello(ctx context.Context, req *HelloReq) (rsp *HelloRsp, err error) {
    log.Printf("client: Hello(%s)", req.Msg)

    rsp = &HelloRsp{}
    rsp.Code = 0;
    rsp.Msg = "";
    return rsp, nil
}