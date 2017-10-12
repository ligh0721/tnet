package tnet

import (
	"log"
	"net"
	"sync"
	"time"
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
	lstn        *net.TCPListener
	Addr        string
	ConnMap     *sync.Map
	connWg      sync.WaitGroup
	ReadBufSize int

	Ext interface{}

	// Listener 监听成功后调用，如果返回 err != nil 则服务器会退出
	OnListenSuccCallback func(self *TcpServer) (err error)

	// 有新连接接入后调用，返回值意义如下：
	// ok: 如果为 false 该连接将会关闭
	// ReadSize: conn 希望连接读取的字节数，如果设置为0，则不会提供一个 self.connReadHandler 例程来读取数据，也就是说可以在 OnAcceptConnCallback 中自定义处理例程
	// connExt: 为 conn 扩展的字段，将会传递到 TCPConnEx 结构中
	OnAcceptConnCallback func(self *TcpServer, conn *net.TCPConn, connId uint32) (ok bool, readSize int, connExt interface{})

	// 连接收到数据后调用，len(data) <= conn.ReadSize，可以在回调中重新设置 conn.ReadSize 来调整下一次期望收到数据的长度
	// 返回值 ok 为 false 将清理并关闭该连接
	OnHandleConnDataCallback func(self *TcpServer, conn *TCPConnEx, connId uint32, data []byte) (ok bool)

	// 关闭连接时调用
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
		log.Printf("close conn(%d)", connId)
		log.Printf("stop conn(%d) handler", connId)
		self.connWg.Done()
	}()

	log.Printf("start conn(%d) handler", connId)

	buf := make([]byte, self.ReadBufSize)
	for {
		n, err := conn.Read(buf[:conn.ReadSize])
		if err != nil {
			log.Printf("conn(%d), %s", connId, err.Error())
			break
		}

		data := buf[:n]
		if self.OnHandleConnDataCallback != nil && !self.OnHandleConnDataCallback(self, conn, connId, data) {
			break
		}
	}
}

func (self *TcpServer) Start() (err error) {
	defer func() {
		if self.lstn != nil {
			self.lstn.Close()
			log.Println("close listener")
		}
		log.Println("stop server")
	}()

	log.Println("start server")
	log.Printf("resolve addr(%s)", self.Addr)
	addr, err := net.ResolveTCPAddr("tcp", self.Addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	log.Printf("listen on %s", addr.String())
	self.lstn, err = net.ListenTCP("tcp", addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	if self.OnListenSuccCallback != nil {
		if err = self.OnListenSuccCallback(self); err != nil {
			return err
		}
	}

	var connId uint32 = 0
	for {
		conn, err := self.lstn.AcceptTCP()
		if err != nil {
			log.Println(err.Error())
			return err
		}
		log.Printf("new conn(%d) from %s", connId, conn.RemoteAddr().String())

		connId++
		if self.OnAcceptConnCallback != nil {
			if ok, readSize, ext := self.OnAcceptConnCallback(self, conn, connId); ok {
				connx := &TCPConnEx{*conn, readSize, ext}
				self.ConnMap.Store(connId, connx)
				self.connWg.Add(1)
				if readSize > 0 {
					// ReadSize > 0 的时候走正常处理函数
					go self.connReadHandler(connx, connId)
				}
			} else {
				connId--
				conn.Close()
				log.Printf("close conn(%d)", connId)
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
	OnDialCallback func(self *TcpClient, conn *net.TCPConn) (ok bool, readSize int, connExt interface{})

	// 连接收到数据后调用，len(data) <= conn.ReadSize，可以在回调中重新设置 conn.ReadSize 来调整下一次期望收到数据的长度
	// 返回值 ok 为 false 将清理并关闭该连接
	OnHandleConnDataCallback func(self *TcpClient, conn *TCPConnEx, data []byte) (ok bool)

	// 关闭连接时调用
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
		log.Println("close conn")
		log.Println("stop conn handler")
	}()

	log.Println("start conn handler")
	buf := make([]byte, self.ReadBufSize)
	for {
		n, err := conn.Read(buf[:conn.ReadSize])
		if err != nil {
			log.Printf("%s", err.Error())
			break
		}

		data := buf[:n]
		if self.OnHandleConnDataCallback != nil && !self.OnHandleConnDataCallback(self, conn, data) {
			break
		}
	}
}

func (self *TcpClient) Start() (err error) {
	log.Println("start client")
	log.Printf("resolve addr(%s)", self.Addr)
	addr, err := net.ResolveTCPAddr("tcp", self.Addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	retryTimesLeft := self.MaxRetry
	for {
		log.Printf("dial to %s", addr.String())
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

		log.Println("conn is established")
		retryTimesLeft = self.MaxRetry
		// 连接成功
		if self.OnDialCallback != nil {
			ok, readSize, ext := self.OnDialCallback(self, conn)
			if !ok {
				conn.Close()
				log.Println("close conn")
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

	log.Println("stop client")
	return nil
}
