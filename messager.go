package tnet

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	"net/http"
	"time"
)

type MessageServer struct {
	db       *sql.DB
	idWorker *IdWorker
}

type MessageProto struct {
	App  string
	Key  string
	Data interface{}
}

func (self *MessageServer) messageHandler(w http.ResponseWriter, r *http.Request) {
	raw, _ := ioutil.ReadAll(r.Body)
	var payload *MessageProto
	err := json.Unmarshal(raw, &payload)
	if err != nil {
		println(err.Error())
		w.Write([]byte(err.Error()))
		return
	}

	stmt, err := self.db.Prepare("INSERT `message` (`id`, `time`, `app`, `key`, `data`) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		println(err.Error())
		w.Write([]byte(err.Error()))
		return
	}

	id, ts, err := self.idWorker.NextId()
	if err != nil {
		println(err.Error())
		w.Write([]byte(err.Error()))
		return
	}
	tm := time.Unix(ts/1000, 0).Format("2006-01-02 15:04:05")
	jsStr, err := json.Marshal(payload.Data)
	if err != nil {
		println(err.Error())
		w.Write([]byte(err.Error()))
		return
	}

	res, err := stmt.Exec(id, tm, payload.App, payload.Key, string(jsStr))
	if err != nil {
		println(err.Error())
		w.Write([]byte(err.Error()))
		return
	}

	id, err = res.LastInsertId()
	if err != nil {
		println(err.Error())
		w.Write([]byte(err.Error()))
		return
	}
	fmt.Println(id)
}

func StartMessagerServer(addr string) (obj *MessageServer, err error) {
	obj = new(MessageServer)
	db, err := sql.Open("mysql", "t5w0rd:753951@tcp(localhost:3306)/messager?charset=utf8")
	if err != nil {
		println(err.Error())
		return nil, err
	}
	obj.db = db
	obj.idWorker, _ = NewIdWorker(0)
	http.HandleFunc("/", obj.messageHandler)
	http.ListenAndServe(addr, nil)
	return obj, nil
}
