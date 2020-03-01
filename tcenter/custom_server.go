package tcenter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"git.tutils.com/tutils/tnet"
)

type CustomTCenterServer struct {
	svr                     *tnet.TcpServer
	Addr                    string
	Seri                    tnet.Serializer
	OnHandleMessageCallback func(self *CustomTCenterServer, cmd string, payload []byte)
}

type CustomTCenterConnExt struct {
	slde *tnet.Slde
}

func NewCustomTCenterServer() (obj *CustomTCenterServer) {
	obj = &CustomTCenterServer{}
	svr := tnet.NewTcpServer()
	svr.Ext = obj
	svr.OnListenSuccCallback = onServerListenSuccCallback
	svr.OnAcceptConnCallback = onServerAcceptConnCallback
	svr.OnHandleConnDataCallback = onServerHandleConnDataCallback
	svr.OnCloseConnCallback = onServerCloseConnCallback
	obj.svr = svr
	obj.Seri = NewPbSeri()
	return obj
}

func unused(interface{}) {
	log.Printf("unused var")
}

func (self *CustomTCenterServer) Start() {
	self.svr.Addr = self.Addr
	self.svr.Start()
}

func (self *CustomTCenterServer) SendCmd(connId int32, cmd string, req interface{}, rsp interface{}) (err error) {
	encodeddata, err := encodeTCenterData(cmd, self.Seri, req)
	if err != nil {
		return err
	}
	conn := self.svr.PeekConn(connId)
	if conn != nil {
		return errors.New(fmt.Sprintf("invalid connId(%d)", connId))
	}
	conn.Write(encodeddata)
	decodeddata, err := tnet.DecodeToBytesFromSldeReader(conn)
	if err != nil {
		return err
	}

	err = decodeTCenterData(cmd, self.Seri, decodeddata, rsp)
	if err != nil {
		return err
	}

	return nil
}

func onServerListenSuccCallback(self *tnet.TcpServer, lstn *net.TCPListener) (ok bool) {
	ext := self.Ext.(*CustomTCenterServer)
	unused(ext)
	return true
}

func onServerAcceptConnCallback(self *tnet.TcpServer, conn *net.TCPConn, connId uint32) (ok bool, readSize int, connExt interface{}) {
	connExt = &CustomTCenterConnExt{tnet.NewSlde()}
	//ext := self.Ext.(*CustomTCenterServer)
	//self.ConnMap.Range(func(key, value interface{}) bool {
	//    k := key.(uint32)
	//    v := value.(*tnet.TCPConnEx)
	//    println(k, v.RemoteAddr().String())
	//    return true;
	//})

	return true, tnet.SLDE_HEADER_SIZE, connExt
}

func onServerHandleConnDataCallback(self *tnet.TcpServer, conn *tnet.TCPConnEx, connId uint32, data []byte) (ok bool) {
	connExt := conn.Ext.(*CustomTCenterConnExt)
	ext := self.Ext.(*CustomTCenterServer)
	left, err := connExt.slde.WriteAndGetNextToWrite(data)
	if err != nil {
		panic(err)
	}
	if left > 0 {
		conn.ReadSize = left
		//log.Printf("slde:left(%d) > 0", elft)
		return true
	} else if left < 0 {
		panic(errors.New("left < 0"))
	}

	// left == 0
	conn.ReadSize = tnet.SLDE_HEADER_SIZE
	decodeddata, err := connExt.slde.DecodeAndReset()
	if err != nil {
		panic(err)
	}
	buf := bytes.NewBuffer(decodeddata)

	var cmdLen uint8
	binary.Read(buf, binary.BigEndian, &cmdLen)
	cmdBytes := make([]byte, cmdLen)
	binary.Read(buf, binary.BigEndian, &cmdBytes)
	cmd := string(cmdBytes)

	if ext.OnHandleMessageCallback != nil {
		ext.OnHandleMessageCallback(ext, cmd, buf.Bytes())
	}

	return true
}

func onServerCloseConnCallback(self *tnet.TcpServer, conn *tnet.TCPConnEx, connId uint32) {
}
