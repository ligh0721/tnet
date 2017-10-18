package tnet

import (
	"log"
	"net"
	"sync"
	"time"
	"errors"
)

const (
	read_buf_size int = 0xffff
)

type TCPConnEx struct {
	net.TCPConn
	ReadSize int
	Ext      interface{}
}

type TcpServer struct {
	Addr        string
	ConnMap     *sync.Map
	connWg      sync.WaitGroup
	ReadBufSize int
	Ext         interface{}

	// Listener 监听成功后调用，如果返回 false 则服务器会退出
	// func(self *tnet.TcpServer, lstn *net.TCPListener) (ok bool) {}
	OnListenSuccCallback func(self *TcpServer, lstn *net.TCPListener) (ok bool)

	// 有新连接接入后调用，返回值意义如下：
	// ok: 如果为 false 该连接将会关闭
	// ReadSize: conn 希望连接读取的字节数，如果设置为0，则不会提供一个 self.connReadHandler 例程来读取数据，也就是说可以在 OnAcceptConnCallback 中自定义处理例程
	// connExt: 为 conn 扩展的字段，将会传递到 TCPConnEx 结构中
	// func(self *tnet.TcpServer, conn *net.TCPConn, connId uint32) (ok bool, readSize int, connExt interface{}) {}
	OnAcceptConnCallback func(self *TcpServer, conn *net.TCPConn, connId uint32) (ok bool, readSize int, connExt interface{})

	// 连接收到数据后调用，len(data) <= conn.ReadSize，可以在回调中重新设置 conn.ReadSize 来调整下一次期望收到数据的长度
	// 返回值 ok 为 false 将清理并关闭该连接
	// func(self *tnet.TcpServer, conn *tnet.TCPConnEx, connId uint32, data []byte) (ok bool) {}
	OnHandleConnDataCallback func(self *TcpServer, conn *TCPConnEx, connId uint32, data []byte) (ok bool)

	// 关闭连接时调用
	// func(self *tnet.TcpServer, conn *tnet.TCPConnEx, connId uint32) {}
	OnCloseConnCallback func(self *TcpServer, conn *TCPConnEx, connId uint32)
}

func NewTcpServer() (obj *TcpServer) {
	obj = new(TcpServer)
	obj.ConnMap = new(sync.Map)
	obj.ReadBufSize = read_buf_size
	return obj
}

func (self *TcpServer) connReadHandler(conn *TCPConnEx, connId uint32) {
	defer func() {
		if self.OnCloseConnCallback != nil {
			self.OnCloseConnCallback(self, conn, connId)
		}
		self.ConnMap.Delete(connId)
		conn.Close()
		log.Printf("close TCP conn(%d)", connId)
		log.Printf("stop TCP conn(%d) handler", connId)
		self.connWg.Done()
	}()

	log.Printf("start TCP conn(%d) handler", connId)

	buf := make([]byte, self.ReadBufSize)
	for {
		n, err0 := conn.Read(buf[:conn.ReadSize])
		if n > 0 {
			data := buf[:n]
			if self.OnHandleConnDataCallback != nil && !self.OnHandleConnDataCallback(self, conn, connId, data) {
				log.Printf("OnHandleConnDataCallback return false")
				break
			}
		}

		if err0 != nil {
			log.Printf("TCP conn(%d), %s", connId, err0.Error())
			break
		}
	}
}

func (self *TcpServer) Start() (err error) {
	var lstn *net.TCPListener
	defer func() {
		if lstn != nil {
			lstn.Close()
			log.Println("close TCP listener")
		}
		log.Println("stop TCP server")
	}()

	log.Println("start TCP server")
	log.Printf("resolve TCP addr(%s)", self.Addr)
	addr, err := net.ResolveTCPAddr("tcp", self.Addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	log.Printf("listen on TCP %s", addr.String())
	lstn, err = net.ListenTCP("tcp", addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	if self.OnListenSuccCallback != nil {
		if ok := self.OnListenSuccCallback(self, lstn); !ok {
			return errors.New("OnListenSuccCallback return false")
		}
	}

	var connId uint32 = 0
	for {
		conn, err := lstn.AcceptTCP()
		if err != nil {
			log.Println(err.Error())
			return err
		}

		connId++
		log.Printf("new TCP conn(%d) from %s", connId, conn.RemoteAddr().String())
		if self.OnAcceptConnCallback != nil {
			if ok, readSize, ext := self.OnAcceptConnCallback(self, conn, connId); ok {
				connx := &TCPConnEx{*conn, readSize, ext}
				self.ConnMap.Store(connId, connx)
				self.connWg.Add(1)
				if self.ReadBufSize > 0 && readSize > 0 {
					// ReadSize > 0 的时候走正常处理函数
					go self.connReadHandler(connx, connId)
				}
			} else {
				connId--
				conn.Close()
				log.Printf("close TCP conn(%d)", connId)
			}
		} else {
			connx := &TCPConnEx{*conn, self.ReadBufSize, nil}
			self.ConnMap.Store(connId, connx)
			self.connWg.Add(1)
			go self.connReadHandler(connx, connId)
		}
	}

	return nil
}

type TcpClient struct {
	Addr        string
	RetryDelay  time.Duration // >= 0
	MaxRetry    int           // -1: infine; 0: no retry
	ReadBufSize int
	Ext interface{}

	// 连接成功后调用，返回值意义如下
	// ok: 如果为 false 该连接将会关闭
	// ReadSize: conn 希望连接读取的字节数
	// connExt: 为 conn 扩展的字段，将会传递到 TCPConnEx 结构中
	// func(self *tnet.TcpClient, conn *net.TCPConn) (ok bool, readSize int, connExt interface{}) {}
	OnDialCallback func(self *TcpClient, conn *net.TCPConn) (ok bool, readSize int, connExt interface{})

	// 连接收到数据后调用，len(data) <= conn.ReadSize，可以在回调中重新设置 conn.ReadSize 来调整下一次期望收到数据的长度
	// 返回值 ok 为 false 将清理并关闭该连接
	// func(self *tnet.TcpClient, conn *tnet.TCPConnEx, data []byte) (ok bool) {}
	OnHandleConnDataCallback func(self *TcpClient, conn *TCPConnEx, data []byte) (ok bool)

	// 关闭连接时调用
	// func(self *tnet.TcpClient, conn *tnet.TCPConnEx) {}
	OnCloseConnCallback func(self *TcpClient, conn *TCPConnEx)
}

func NewTcpClient() (obj *TcpClient) {
	obj = new(TcpClient)
	obj.RetryDelay = 0
	obj.MaxRetry = 0
	obj.ReadBufSize = read_buf_size
	return obj
}

func (self *TcpClient) connReadHandler(conn *TCPConnEx) {
	defer func() {
		if self.OnCloseConnCallback != nil {
			self.OnCloseConnCallback(self, conn)
		}
		conn.Close()
		log.Println("close TCP conn")
		log.Println("stop TCP conn handler")
	}()

	log.Println("start TCP conn handler")
	buf := make([]byte, self.ReadBufSize)
	for {
		n, err0 := conn.Read(buf[:conn.ReadSize])
		if n > 0 {
			data := buf[:n]
			if self.OnHandleConnDataCallback != nil && !self.OnHandleConnDataCallback(self, conn, data) {
				log.Printf("OnHandleConnDataCallback return false")
				break
			}
		}

		if err0 != nil {
			log.Printf("%s", err0.Error())
			break
		}
	}
}

func (self *TcpClient) Start() (err error) {
	log.Println("start TCP client")
	log.Printf("resolve TCP addr(%s)", self.Addr)
	addr, err := net.ResolveTCPAddr("tcp", self.Addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	retryTimesLeft := self.MaxRetry
	for {
		log.Printf("dial to TCP %s", addr.String())
		tm := time.Now()
		conn, err := net.DialTCP("tcp", nil, addr)
		if err != nil {
			log.Printf("%s, retry times(%d)", err.Error(), retryTimesLeft)
			if retryTimesLeft == 0 {
				break
			} else if retryTimesLeft > 0 {
				retryTimesLeft--
			}

			left := self.RetryDelay - time.Now().Sub(tm)
			if left > 0 {
				log.Printf("wait for %dms", left/1e6)
				time.Sleep(left)
			}
			continue
		}

		log.Println("TCP conn is established")
		retryTimesLeft = self.MaxRetry
		// 连接成功
		if self.OnDialCallback != nil {
			ok, readSize, ext := self.OnDialCallback(self, conn)
			if !ok {
				conn.Close()
				log.Println("close TCP conn")
				break
			}
			connx := &TCPConnEx{*conn, readSize, ext}
			if readSize > 0 {
				// ReadSize > 0 的时候走正常处理函数
				self.connReadHandler(connx)
			}
		} else {
			connx := &TCPConnEx{*conn, self.ReadBufSize, nil}
			self.connReadHandler(connx)
		}
	}

	log.Println("stop TCP client")
	return nil
}

const (
	udp_peer_mode_defualt int = 0
	udp_peer_mode_server int = 1
	udp_peer_mode_client int = 2
)

type UdpPeer struct {
	Addr        string
	ReadBufSize int
	mode		int
	Ext interface{}

	// Listener 监听成功后调用，如果返回 false 则服务器会退出
	// func(self *tnet.UdpPeer, conn *net.UDPConn) (ok bool) {}
	OnListenSuccCallback func(self *UdpPeer, conn *net.UDPConn) (ok bool)

	// 连接成功后调用，如果返回 false 则服务器会退出
	// func(self *tnet.UdpPeer, conn *net.UDPConn) (ok bool) {}
	OnDialCallback func(self *UdpPeer, conn *net.UDPConn) (ok bool)

	// 连接收到数据后调用
	// 返回值 ok 为 false 将清理并关闭该连接
	// func(self *tnet.UdpPeer, conn *net.UDPConn, addr *net.UDPAddr, data []byte) (ok bool) {}
	OnHandleConnDataCallback func(self *UdpPeer, conn *net.UDPConn, addr *net.UDPAddr, data []byte) (ok bool)

	// 关闭连接时调用
	// func(self *tnet.UdpPeer, conn *net.UDPConn) {}
	OnCloseConnCallback func(self *UdpPeer, conn *net.UDPConn)
}

func NewUdpPeer() (obj *UdpPeer) {
	obj = new(UdpPeer)
	obj.mode = udp_peer_mode_defualt
	obj.ReadBufSize = read_buf_size
	return obj
}

func NewUdpClient() (obj *UdpPeer) {
	obj = new(UdpPeer)
	obj.mode = udp_peer_mode_client
	obj.ReadBufSize = read_buf_size
	return obj
}

func NewUdpServer() (obj *UdpPeer) {
	obj = new(UdpPeer)
	obj.mode = udp_peer_mode_server
	obj.ReadBufSize = read_buf_size
	return obj
}

func (self *UdpPeer) connReadHandler(conn *net.UDPConn) {
	defer func() {
		log.Println("stop UDP conn handler")
	}()

	log.Println("start UDP conn handler")
	buf := make([]byte, self.ReadBufSize)
	var err0 error
	var n int
	var addr *net.UDPAddr
	if self.mode == udp_peer_mode_client {
		caddr := conn.RemoteAddr()
		addr, _ = net.ResolveUDPAddr(caddr.Network(), caddr.String())
	}
	for {
		if self.mode == udp_peer_mode_client {
			n, err0 = conn.Read(buf)
		} else {
			n, addr, err0 = conn.ReadFromUDP(buf)
		}

		if n > 0 {
			data := buf[:n]
			if self.OnHandleConnDataCallback != nil && !self.OnHandleConnDataCallback(self, conn, addr, data) {
				log.Printf("OnHandleConnDataCallback return false")
				break
			}
		}

		if err0 != nil {
			log.Printf("%s", err0.Error())
			break
		}
	}
}

func (self *UdpPeer) Start() (err error) {
	var conn *net.UDPConn
	defer func() {
		if conn != nil {
			if self.OnCloseConnCallback != nil {
				self.OnCloseConnCallback(self, conn)
			}
			conn.Close()
			log.Println("close UDP conn")
		}
		log.Println("stop UDP server")
	}()
	log.Println("start UDP server")
	log.Printf("resolve UDP addr(%s)", self.Addr)
	addr, err := net.ResolveUDPAddr("udp", self.Addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	if self.mode == udp_peer_mode_server {
		log.Printf("listen on UDP %s", addr.String())
		conn, err = net.ListenUDP("udp", addr)
		if err != nil {
			log.Println(err.Error())
			return err
		}

		if self.OnListenSuccCallback != nil {
			if ok := self.OnListenSuccCallback(self, conn); !ok {
				return errors.New("OnListenSuccCallback return false")
			}
		}
	} else if (self.mode == udp_peer_mode_client) {
		log.Printf("dial to UDP %s", addr.String())
		conn, err = net.DialUDP("udp", nil, addr)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		//log.Println("UDP conn is established")

		if self.OnDialCallback != nil {
			if ok := self.OnDialCallback(self, conn); !ok {
				return nil
			}
		}
	}

	if self.ReadBufSize > 0 {
		self.connReadHandler(conn)
	}

	log.Println("stop UDP client")
	return nil
}