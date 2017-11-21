package tnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"sync"
)

/* encrypt connection
===================================
conn \                       / conn
conn -<-> proxy <-> agent <->- conn
conn /                       \ conn
===================================
*/

const (
	cmd_connect uint16 = 0
	cmd_data    uint16 = 1
	cmd_close   uint16 = 2

	server_mode_proxy = 0
	server_mode_agent = 1

	max_tcp_read               = 0xffff
	max_peer_conn_op_chan_size = 10
	max_close_notify_chan_size = 1024
)

type connChanItem struct {
	cmd    uint16
	reader io.Reader
}

type EncryptTunPeer struct {
	// 所有线程都有用到，初始化后不会改动 或 线程安全
	peer                *net.TCPConn
	addr                *net.TCPAddr
	mode                byte
	connChanMap         *sync.Map // map[uint32] chan connChanItem
	connCloseNotifyChan chan uint32
	wg                  sync.WaitGroup
	lstn                *net.TCPListener
}

func NewEncryptConnProxy(peer *net.TCPConn, laddr string) (obj *EncryptTunPeer) {
	obj = new(EncryptTunPeer)
	obj.peer = peer
	log.Printf("resolve addr(%s)", laddr)
	obj.addr, _ = net.ResolveTCPAddr("tcp", laddr)
	obj.mode = server_mode_proxy
	obj.connChanMap = new(sync.Map)
	obj.connCloseNotifyChan = make(chan uint32, max_close_notify_chan_size)
	return obj
}

func NewEncryptConnAgent(peer *net.TCPConn, raddr string) (obj *EncryptTunPeer) {
	obj = new(EncryptTunPeer)
	obj.peer = peer
	log.Printf("resolve addr(%s)", raddr)
	obj.addr, _ = net.ResolveTCPAddr("tcp", raddr)
	obj.mode = server_mode_agent
	obj.connChanMap = new(sync.Map)
	obj.connCloseNotifyChan = make(chan uint32, max_close_notify_chan_size)
	return obj
}

// connect = cmd:uint16 + connId:uint32
func packConnect(connId uint32) (ret []byte) {
	payload := bytes.NewBuffer([]byte{})
	binary.Write(payload, binary.BigEndian, cmd_connect)
	binary.Write(payload, binary.BigEndian, connId)
	ret, _ = EncodeToSldeDataFromBytes(payload.Bytes())
	//log.Println("pack", ret)
	return ret
}

// write = cmd:uint16 + connId:uint32 + dataLen:uint32 + data:string(dataLen)
func packData(connId uint32, data []byte) (ret []byte) {
	payload := bytes.NewBuffer([]byte{})
	binary.Write(payload, binary.BigEndian, cmd_data)
	binary.Write(payload, binary.BigEndian, connId)
	dataLen := uint32(len(data))
	binary.Write(payload, binary.BigEndian, dataLen)
	binary.Write(payload, binary.BigEndian, data)
	ret, _ = EncodeToSldeDataFromBytes(payload.Bytes())
	//log.Println("pack", ret)
	return ret
}

// close cmd:uint16 + connId:uint32
func packClose(connId uint32) (ret []byte) {
	payload := bytes.NewBuffer([]byte{})
	binary.Write(payload, binary.BigEndian, cmd_close)
	binary.Write(payload, binary.BigEndian, connId)
	ret, _ = EncodeToSldeDataFromBytes(payload.Bytes())
	//log.Println("pack", ret)
	return ret
}

// 解码出 cmd 并且返回一个用于继续解码的 io.Reader
func unpackCmd(data []byte) (cmd uint16, reader io.Reader) {
	reader = bytes.NewBuffer(data)
	binary.Read(reader, binary.BigEndian, &cmd)
	return cmd, reader
}

// 解码 connId
func unpackConnId(reader io.Reader) (connId uint32) {
	binary.Read(reader, binary.BigEndian, &connId)
	return connId
}

func unpackConnect(reader io.Reader) {
}

func unpackData(reader io.Reader) (data []byte) {
	var dataLen uint32
	binary.Read(reader, binary.BigEndian, &dataLen)
	data = make([]byte, dataLen)
	binary.Read(reader, binary.BigEndian, data)
	return data
}

func unpackClose(reader io.Reader) {
}

func (self *EncryptTunPeer) notifyToCloseChan(c chan connChanItem, connId uint32) {
_for:
	for {
		select {
		case _, ok := <-c:
			if !ok {
				break _for
			}
			// drop
			log.Printf("drop conn(%d) chan datas", connId)
		default:
			log.Printf("@@ send notify chan, conn(%d)", connId)
			self.connCloseNotifyChan <- connId
			log.Printf("## sent notify chan, conn(%d), finished", connId)
			break _for
		}
	}
}

func (self *EncryptTunPeer) clean() {
	log.Println("clean")
	todel := self.connChanMap
	self.connChanMap = new(sync.Map)
	todel.Range(func(k, v interface{}) bool {
		connId := k.(uint32)
		connChan := v.(chan connChanItem)
		log.Printf("cleaning, close conn(%d) chan", connId)
		close(connChan)
		return true
	})

_for:
	for {
		select {
		case _, ok := <-self.connCloseNotifyChan:
			if !ok {
				break _for
			}
		default:
			break _for
		}
	}
	self.connCloseNotifyChan = make(chan uint32, max_close_notify_chan_size)

	log.Println("wait for stopping all of conn handler and peer conn op hander")
	self.wg.Wait()
	log.Println("all of conn handler and peer conn op hander stopped")

	if self.mode == server_mode_proxy {
		log.Printf("stop listener(%s)", self.addr.String())
		self.lstn.Close()
	}
}

// 连接处理循环
func (self *EncryptTunPeer) startConnHandler(conn *net.TCPConn, connId uint32) {
	self.wg.Add(1)
	log.Printf("start conn(%d) handler", connId)
	buf := make([]byte, max_tcp_read)
	for {
		n, err0 := conn.Read(buf)
		if n > 0 {
			// tell to (connect and )send data
			log.Printf("send conn(%d) op: senddata", connId)
			protodata := packData(connId, buf[:n])
			self.peer.Write(protodata)
		}

		if err0 != nil {
			log.Println(err0.Error())
			break
		}
	}

	log.Printf("stop conn(%d) handler", connId)
	if v, ok := self.connChanMap.Load(connId); ok {
		// 来自 conn 的关闭
		log.Printf("send conn(%d) op: close", connId)
		protodata := packClose(connId)
		self.peer.Write(protodata)
		connChan := v.(chan connChanItem)
		log.Printf("conn EOF, notify to close conn(%d) chan", connId)
		//safeClose(connChan)
		self.notifyToCloseChan(connChan, connId)
	}
	self.wg.Done()
}

// 处理远端 peer 发送过来的请求
func (self *EncryptTunPeer) goStartPeerConnOpHandler(conn *net.TCPConn, connId uint32) (connChan chan connChanItem) {
	log.Printf("start peer conn(%d) op handler", connId)
	connChan = make(chan connChanItem, max_peer_conn_op_chan_size)
	self.connChanMap.Store(connId, connChan)

	go func() {
		self.wg.Add(1)

		var ok bool
		var item connChanItem

	_for:
		for {
			item, ok = <-connChan
			if !ok {
				log.Printf("conn(%d) chan is closed", connId)
				//self.connChanMap.Delete(connId)
				if conn != nil {
					conn.Close()
					log.Printf("conn(%d) is closing", connId)
				}
				break
			}

			switch item.cmd {
			case cmd_connect:
				unpackConnect(item.reader)
				// agent
				var err error
				log.Printf("conn(%d) dial(%s)", connId, self.addr.String())
				conn, err = net.DialTCP("tcp", nil, self.addr)
				if err != nil {
					// tell to close
					log.Println(err.Error())
					log.Printf("send conn(%d) op: close", connId)
					protodata := packClose(connId)
					self.peer.Write(protodata)
					log.Printf("dial(%s) err, notify to close conn(%d) chan", self.addr.String(), connId)
					//safeClose(connChan)
					self.notifyToCloseChan(connChan, connId)
				} else {
					go self.startConnHandler(conn, connId)
				}

			case cmd_data:
				data := unpackData(item.reader)
				conn.Write(data)

			case cmd_close:
				unpackClose(item.reader)
				//self.connChanMap.Delete(connId)
				conn.Close()
				log.Printf("conn(%d) is closing", connId)
				log.Printf("peer op, notify to close conn(%d) chan", connId)
				//safeClose(connChan)
				self.notifyToCloseChan(connChan, connId)
				break _for
			}
		}
		log.Printf("stop peer conn(%d) op handler", connId)
		self.wg.Done()
	}()

	return connChan
}

// 连接操作序列化
func (self *EncryptTunPeer) dispatchPeerConnOp(cmd uint16, reader io.Reader) {
	connId := unpackConnId(reader)
	var connChan chan connChanItem
	if cmd == cmd_connect {
		connChan = self.goStartPeerConnOpHandler(nil, connId)
	} else if v, ok := self.connChanMap.Load(connId); ok {
		connChan = v.(chan connChanItem)
	} else {
		log.Printf("invalid dispatch, connId(%d)", connId)
		return
	}
	log.Printf("@@ serialize op conn(%d), send conn chan", connId)
	connChan <- connChanItem{cmd, reader}
	log.Printf("## serialize op conn(%d), sent conn chan, finished", connId)
}

// 主连接处理循环
func (self *EncryptTunPeer) startPeerHandler() {
	log.Println("start peer handler")
	buf := make([]byte, max_tcp_read)
	slde := NewSlde()
	sldeleft := SLDE_HEADER_SIZE
	var err error
	for {
		//log.Printf("peer will read %d bytes", sldeleft)
		log.Println("@@@@@ peer read")
		n, err0 := self.peer.Read(buf[:sldeleft])
		log.Println("##### peer read finished")
		if n > 0 {
			sldeleft, err = slde.WriteAndGetNextToWrite(buf[:n])
			if err != nil {
				log.Println(err.Error())
				// close all connection
				self.clean()
				break
			}

			if sldeleft == 0 {
				// 一个协议包接收完成，根据connId将slde加入对应的待处理队列
				recvdata, err := slde.Decode()
				if err != nil {
					log.Println(err.Error())
					// close all connection
					self.clean()
					break
				}
				slde = NewSlde()
				sldeleft = SLDE_HEADER_SIZE
				log.Println("slde recv complete")

				select {
				case connId, ok := <-self.connCloseNotifyChan:
					if ok {
						if v, ok := self.connChanMap.Load(connId); ok {
							connChan := v.(chan connChanItem)
							log.Printf("close conn(%d) chan", connId)
							self.connChanMap.Delete(connId)
							close(connChan)
						}
					}
				default:
				}

				cmd, recvReader := unpackCmd(recvdata)
				switch cmd {
				case cmd_connect:
					log.Println("dispatch cmd: connect")
					self.dispatchPeerConnOp(cmd, recvReader)
				case cmd_data:
					log.Println("dispatch cmd: senddata")
					self.dispatchPeerConnOp(cmd, recvReader)
				case cmd_close:
					log.Println("dispatch cmd: close")
					self.dispatchPeerConnOp(cmd, recvReader)
				}
			} else if sldeleft > max_tcp_read {
				sldeleft = max_tcp_read
			}
		}

		if err0 != nil {
			log.Println(err0.Error())
			// close all connection
			self.clean()
			break
		}
	}

	log.Println("stop peer handler")
}

func (self *EncryptTunPeer) startProxy() (err error) {
	log.Println("start proxy")
	log.Printf("start listener(%s)", self.addr.String())
	self.lstn, err = net.ListenTCP("tcp", self.addr)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	defer self.lstn.Close()

	go self.startPeerHandler()

	var connId uint32 = 0
	for {
		conn, err := self.lstn.AcceptTCP()
		if err != nil {
			log.Println(err.Error())
			break
		}

		connId += 1
		self.goStartPeerConnOpHandler(conn, connId)
		log.Printf("send conn(%d) op: connect", connId)
		data := packConnect(connId)
		self.peer.Write(data)
		go self.startConnHandler(conn, connId)
	}

	return err
}

func (self *EncryptTunPeer) startAgent() (err error) {
	log.Println("start agent")
	self.startPeerHandler()
	return nil
}

// 启动proxy
func (self *EncryptTunPeer) Start() (err error) {
	if self.mode == server_mode_proxy {
		return self.startProxy()
	} else if self.mode == server_mode_agent {
		return self.startAgent()
	}
	return errors.New("unsupported mode")
}
