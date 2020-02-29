package messager

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
	"strconv"
	"log"
	"tnet"
)

type MessageServer struct {
	db       *sql.DB
	idWorker *tnet.IdWorker
}

type MessageProto struct {
	Ver  int
	App  string
	Key  string
	Data interface{}
}

type RspProto struct {
	Ver  int
	Code int
	Msg  string
	Data interface{}
}

type EmptyRspData struct {
}

type MessageRspProto struct {
	Id string
}

func (self *MessageServer) messageHandler(w http.ResponseWriter, r *http.Request) {
	raw, _ := ioutil.ReadAll(r.Body)
	var payload *MessageProto
	err := json.Unmarshal(raw, &payload)
	if err != nil {
		log.Printf(err.Error())
		ResponseError(w, 1, 1, err.Error())
		return
	}

	stmt, err := self.db.Prepare("INSERT `message` (`id`, `time`, `app`, `key`, `data`) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		log.Printf(err.Error())
		ResponseError(w, 1, 2, err.Error())
		return
	}

	id, ts, err := self.idWorker.NextId()
	if err != nil {
		log.Printf(err.Error())
		ResponseError(w, 1, 3, err.Error())
		return
	}
	tm := time.Unix(ts/1000, 0).Format("2006-01-02 15:04:05")
	jsStr, err := json.Marshal(payload.Data)
	if err != nil {
		log.Printf(err.Error())
		ResponseError(w, 1, 4, err.Error())
		return
	}

	_, err = stmt.Exec(id, tm, payload.App, payload.Key, string(jsStr))
	if err != nil {
		log.Printf(err.Error())
		ResponseError(w, 1, 5, err.Error())
		return
	}

	idStr := strconv.FormatInt(id, 10)
	rspData := MessageRspProto{idStr}
	ResponseData(w, 1, rspData)
}

func ResponseError(w http.ResponseWriter, ver int, errcode int, errmsg string) {
	w.Header().Add("content-type", "application/json; charset=utf-8")
	rsp := RspProto{
		ver,
		errcode,
		errmsg,
		EmptyRspData{},
	}
	jsStr, err := json.Marshal(rsp)
	if err != nil {
		log.Printf(err.Error())
		return
	}
	w.Write(jsStr)
}

func ResponseData(w http.ResponseWriter, ver int, data interface{}) {
	w.Header().Add("content-type", "application/json; charset=utf-8")
	rsp := RspProto{
		ver,
		0,
		"",
		data,
	}
	jsStr, err := json.Marshal(rsp)
	if err != nil {
		ResponseError(w, ver, -1, err.Error())
		return
	}
	w.Write(jsStr)
}

func StartMessagerServer(addr string) (obj *MessageServer, err error) {
	obj = new(MessageServer)
	db, err := sql.Open("mysql", "t5w0rd:753951@tcp(localhost:3306)/messager?charset=utf8")
	if err != nil {
		println(err.Error())
		return nil, err
	}
	obj.db = db
	obj.idWorker, _ = tnet.NewIdWorker(0)
	sv := http.NewServeMux()
	sv.HandleFunc("/", obj.messageHandler)
	http.ListenAndServe(addr, sv)
	return obj, nil
}
