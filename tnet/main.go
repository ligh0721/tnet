package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jamescun/tuntap"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"
	"git.tutils.com/tutils/tnet"
	"git.tutils.com/tutils/tnet/messager"
	"git.tutils.com/tutils/tnet/tcenter"
	"git.tutils.com/tutils/tnet/tcounter"
	"git.tutils.com/tutils/tnet/tqa"
)

type UdpExt struct {
	first bool
}

func grow(list interface{}) {
	switch list.(type) {
	case *[]int:
		l := list.(*[]int)
		*l = (*l)[:len(*l)+1]
	}
}

func test() {
	conn, _ := net.ListenPacket("udp", ":")
	buf := []byte{0, 0}
	conn.ReadFrom(buf)
}

type UdpTunServerExt struct {
	codec      tnet.CryptCodec
	device     string
	secret     string
	handshaked bool
	maxWrite   int
	itf        tuntap.Interface
	mtx        *sync.RWMutex
	lastAddr   *net.UDPAddr
}

func buildParams() []byte {
	buf := bytes.NewBuffer(make([]byte, 0, bytes.MinRead))
	buf.WriteByte(0x00)
	writeArg := false
	needSep := false
	for _, arg := range os.Args {
		if arg[0] == '-' {
			if needSep {
				buf.WriteByte(' ')
			}
			buf.WriteByte(arg[1])
			writeArg = true
			needSep = true
		} else if writeArg {
			if needSep {
				buf.WriteByte(',')
			}
			buf.WriteString(arg)
		}
	}
	return buf.Bytes()
}

func parseIpPacket(data []byte) {
	buf := bytes.NewBuffer(data)
	var pad32 uint32
	binary.Read(buf, binary.BigEndian, &pad32)
	binary.Read(buf, binary.BigEndian, &pad32)
	binary.Read(buf, binary.BigEndian, &pad32)
	srcIp := net.IP{0, 0, 0, 0}
	dstIp := net.IP{0, 0, 0, 0}
	binary.Read(buf, binary.BigEndian, &srcIp)
	binary.Read(buf, binary.BigEndian, &dstIp)
	log.Printf("%s -> %s", srcIp.String(), dstIp.String())
}

// "\x00m,1400 a,192.168.100.2,32 d,8.8.8.8 r,0.0.0.0,0"
func runUdpTunServer() {
	ext := &UdpTunServerExt{
		tnet.NewZlibXorCodec(19284562),
		os.Args[3],
		os.Args[4],
		false,
		3,
		nil,
		new(sync.RWMutex),
		nil,
	}

	laddr := os.Args[2]
	svr := tnet.NewUdpServer()
	svr.Addr = laddr
	svr.Ext = ext
	svr.OnListenSuccCallback = func(self *tnet.UdpPeer, conn *net.UDPConn) (ok bool) {
		ext := self.Ext.(*UdpTunServerExt)
		var err error
		ext.itf, err = tuntap.Tun(ext.device)
		if err != nil {
			panic(err)
		}

		go func() {
			for {
				buf := make([]byte, 0xffff)
				n, err0 := ext.itf.Read(buf)
				if n > 0 {
					data := buf[:n]
					encodeddata := ext.codec.Encrypt(data)
					ext.mtx.RLock()
					addr := ext.lastAddr
					ext.mtx.RUnlock()
					if ext.lastAddr != nil {
						parseIpPacket(data)
						log.Printf("write to conn %d(%d) bytes", len(encodeddata), len(data))
						_, err := conn.WriteToUDP(encodeddata, addr)
						if err != nil {
							panic(err)
						}
					}
				}
				if err0 != nil {
					panic(err0)
				}
			}
		}()

		return true
	}
	svr.OnHandleConnDataCallback = func(self *tnet.UdpPeer, conn *net.UDPConn, addr *net.UDPAddr, data []byte) (ok bool) {
		ext := self.Ext.(*UdpTunServerExt)
		decodeddata, err := ext.codec.Decrypt(data)
		if err != nil {
			panic(err)
		}

		if decodeddata[0] == 0x00 {
			log.Printf("handshake")
			secret := string(decodeddata[1:])
			if secret == ext.secret {
				log.Printf("secret is ok")
				data := buildParams()
				encodeddata := ext.codec.Encrypt(data)
				for i := 0; i < ext.maxWrite; i++ {
					conn.WriteToUDP(encodeddata, addr)
				}

				ext.mtx.Lock()
				ext.lastAddr = addr
				ext.mtx.Unlock()

				ext.handshaked = true
			} else {
				log.Printf("secret(%s) is wrong", secret)
			}
		} else {
			// parse data
			parseIpPacket(decodeddata)

			log.Printf("write to device %d bytes", len(decodeddata))
			ext.itf.Write(decodeddata)
		}
		return true
	}
	svr.Start()
}

type SvrExt struct {
	lastConnId uint32
}

type ConnExt struct {
	slde      *tnet.Slde
	handshake bool
}

func tunTcpServer() {
	itf, err := tuntap.Tun("tun0")
	if err != nil {
		panic(err)
	}

	svr := tnet.NewTcpServer()
	svr.Addr = "0.0.0.0:2888"
	svr.Ext = &SvrExt{0}
	svr.OnListenSuccCallback = func(self *tnet.TcpServer, lstn *net.TCPListener) (ok bool) {
		go func() {
			buf := make([]byte, 0xffff)
			for {
				n, err0 := itf.Read(buf)
				if n > 0 {
					log.Printf("/dev/tun READ: %d bytes", n)
					svrExt := svr.Ext.(*SvrExt)
					if v, ok := self.ConnMap.Load(svrExt.lastConnId); ok {
						conn := v.(*tnet.TCPConnEx)
						encodeddata, _ := tnet.EncodeToSldeDataFromBytes(buf[:n])
						log.Printf("write to conn %d bytes", len(encodeddata))
						conn.Write(encodeddata)
					} else {
						log.Printf("conn not found, lastConnId: %d", svrExt.lastConnId)
					}
				}
				if err0 != nil {
					log.Printf(err0.Error())
					break
				}
			}
		}()
		return true
	}
	svr.OnAcceptConnCallback = func(self *tnet.TcpServer, conn *net.TCPConn, connId uint32) (ok bool, readSize int, connExt interface{}) {
		svrExt := self.Ext.(*SvrExt)
		svrExt.lastConnId = connId
		connExt = &ConnExt{tnet.NewSlde(), false}
		return true, tnet.SLDE_HEADER_SIZE, connExt
	}
	svr.OnHandleConnDataCallback = func(self *tnet.TcpServer, conn *tnet.TCPConnEx, connId uint32, data []byte) (ok bool) {
		ext := conn.Ext.(*ConnExt)
		left, err := ext.slde.WriteAndGetNextToWrite(data)
		if err != nil {
			panic(err)
		}
		if left > 0 {
			conn.ReadSize = left
			log.Printf("slde:left(%d) > 0", left)
			return true
		} else if left < 0 {
			panic(errors.New("left < 0"))
		}

		// left == 0
		conn.ReadSize = tnet.SLDE_HEADER_SIZE
		decodeddata, err := ext.slde.DecodeAndReset()
		if err != nil {
			panic(err)
		}

		//log.Printf("decodeddata: [% x]", decodeddata)
		if !ext.handshake {
			if string(decodeddata[1:]) == "test" {
				params := []byte("\x00m,1400 a,192.168.100.2,32 d,8.8.8.8 r,0.0.0.0,0")
				encodedcode, _ := tnet.EncodeToSldeDataFromBytes(params)
				conn.Write(encodedcode)
				log.Println("handshake succ!")
				ext.handshake = true
				ext = conn.Ext.(*ConnExt)
				log.Println(ext.handshake)
			} else {
				log.Println("handshake failed!")
				return false
			}
		} else {
			log.Println("write to tun")
			itf.Write(decodeddata)
		}

		return true
	}
	svr.Start()
}

func runProxy() {
	for {
		clt := tnet.NewTcpClient()
		clt.Addr = os.Args[2]
		clt.RetryDelay = 1e9
		clt.MaxRetry = -1
		clt.OnDialCallback = func(self *tnet.TcpClient, conn *net.TCPConn) (ok bool, readSize int, connExt interface{}) {
			proxy := tnet.NewEncryptConnProxy(conn, os.Args[3])
			proxy.Start()
			return false, 0, proxy
		}
		clt.Start()

		time.Sleep(1e9)
	}
}

func runAgent() {
	for {
		svr := tnet.NewTcpServer()
		svr.Addr = os.Args[2]
		svr.OnAcceptConnCallback = func(self *tnet.TcpServer, conn *net.TCPConn, connId uint32) (ok bool, readSize int, connExt interface{}) {
			agent := tnet.NewEncryptConnAgent(conn, os.Args[3])
			log.Printf("@@@@@ OnAcceptConnCallback")
			go agent.Start()
			return true, 0, agent
		}
		svr.Start()

		time.Sleep(1e9)
	}
}

func runTCenterServer() {
	svr := tcenter.NewTCenterServer()
	svr.Addr = os.Args[2]
	svr.Start()
}

func runTCenterClient() {
	clt := tcenter.NewTCenterClient()
	clt.Addr = os.Args[2]
	clt.Start()
	defer clt.Close()
	lastInfoStr := clt.Login()
	var cmd string
	if len(os.Args) > 3 {
		cmd = os.Args[3]
	} else {
		cmd = ""
	}

	switch cmd {
	case "list":
		clt.ListClients()
		break
	default:
		clt.HealthLoop(lastInfoStr)
		break
	}
}

func runTCounterServer() {
	svr := tcounter.NewCounterServer()
	svr.Addr = ":53088"
	svr.DbCfg.Host = "localhost"
	svr.DbCfg.Port = 3306
	svr.DbCfg.Name = "tcounter"
	svr.DbCfg.User = "tcounter"
	svr.DbCfg.Pass = "tcounter"
	svr.HttpAddr = ":51001"

	svr.Start()
}

func runTCounterAgent() {
	agent := tcounter.NewCounterAgentUseUnix("/tmp/tcountera.sock", "tvpsx.tutils.com:53088")
	agent.Start()
}

func runTCounterClient() {
	go func() {
		http.ListenAndServe(":8103", nil)
	}()

	clt := tcounter.NewCounterClientUseUnix("/tmp/tcountera.sock")
	wg := &sync.WaitGroup{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			rnd := rand.New(rand.NewSource(time.Now().Unix()))
			for {
				clt.SendValue(100, int64(rnd.Int()%(time.Now().Hour()+1)))
				time.Sleep(20e6)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	clt.Close()
}

func runQaServer() {
	qas := tqa.NewQaServer()
	qas.HttpAddr = ":8080"
	qas.StaticRoot = "qaroot/static/"
	//qas.ExpiredAnswered = 10e9
	//qas.ExpiredUnanswered = 10e9
	qas.Start()
}

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	args := os.Args
	if len(args) < 2 {
		return
	}
	appType := args[1]
	switch appType {
	case "messager":
		addr := args[2]
		messager.StartMessagerServer(addr)

	case "test":
		test()

	case "proxy":
		runProxy()

	case "agent":
		runAgent()

	case "tun":
		// tun :2889 tun0 tutils -m 1400 -a 192.168.100.2 32 -d 8.8.8.8 -r 0.0.0.0 0
		runUdpTunServer()

	case "tcenters":
		runTCenterServer()

	case "tcenterc":
		runTCenterClient()

	case "tcounters":
		runTCounterServer()

	case "tcountera":
		runTCounterAgent()

	case "tcounterc":
		runTCounterClient()

	case "qas":
		runQaServer()
	}
}
