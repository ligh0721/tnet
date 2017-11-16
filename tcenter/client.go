package tcenter

import (
    "log"
    "os"
    "google.golang.org/grpc"
    "golang.org/x/net/context"
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

    // Contact the server and print out its response.
    msg := "unamed"
    if len(os.Args) > 2 {
        msg = os.Args[2]
    }
    req := &HelloReq{}
    req.Msg = msg
    rsp, err := c.Hello(context.Background(), req)
    if err != nil {
        log.Fatalf("could not req hello: %v", err)
    }
    log.Printf("rsp: code(%d), msg(%s)", rsp.Code, rsp.Msg)
}