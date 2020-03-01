package tqa

import (
	"encoding/base64"
	"encoding/json"
	"git.tutils.com/tutils/tnet"
	"git.tutils.com/tutils/tnet/encoding/mapstructure"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HttpReq struct {
	Ver  int
	Data interface{}
}

type HttpRspCode int

const (
	CodeSuccess          HttpRspCode = 0
	CodeError            HttpRspCode = 1
	CodeQuestionNotFound HttpRspCode = 2
	CodeNoAnswer         HttpRspCode = 3
	CodeAnswered         HttpRspCode = 4
)

type HttpRsp struct {
	Ver  int
	Code HttpRspCode
	Msg  string
	Data interface{}
}

var EMPTY_HTTP_RSP_DATA struct{}

type QuestionAndAnwser struct {
	Title    string
	Question interface{}
	Time     time.Time
	Answer   interface{}
}
type QaServer struct {
	HttpAddr          string
	StaticRoot        string
	ExpiredAnswered   time.Duration
	ExpiredUnanswered time.Duration

	idWorker *tnet.IdWorker
	db       *sync.Map
}

func NewQaServer() (obj *QaServer) {
	obj = new(QaServer)
	obj.idWorker, _ = tnet.NewIdWorker(0)
	obj.db = &sync.Map{}
	obj.ExpiredAnswered = 3600 * 1e9
	obj.ExpiredUnanswered = 3600 * 24 * 1e9
	return obj
}

func (self *QaServer) Start() {
	//err := os.MkdirAll("VerifyCodeImg", 0755)  //生成多级目录
	//if err != nil {
	//    log.Fatalf("%v", err)
	//}
	go self.checkExpired()
	self.serveHttp()
}

const (
	PatternStatic      = "/qa/static/"
	PatternApiAsk      = "/qa/api/ask"
	PatternApiQuery    = "/qa/api/query/"
	PatternApiQuestion = "/qa/api/question/"
	PatternApiAnswer   = "/qa/api/answer/"
	PatternApiList     = "/qa/api/list"
	PatternViewAnswer  = "/qa/view/answer/"
	PatternViewList    = "/qa/view/list"
)

const (
	//PatternLenApiAsk      = len(PatternApiAsk)
	PatternLenApiQuery    = len(PatternApiQuery)
	PatternLenApiQuestion = len(PatternApiQuestion)
	PatternLenApiAnswer   = len(PatternApiAnswer)
	//PatternLenApiList     = len(PatternApiList)
	PatternLenViewAnswer = len(PatternViewAnswer)
)

func (self *QaServer) serveHttp() {
	sv := http.NewServeMux()
	sv.Handle(PatternStatic, http.StripPrefix(PatternStatic, http.FileServer(http.Dir(self.StaticRoot))))
	sv.HandleFunc(PatternApiAsk, self.handleHttpApiAsk)
	sv.HandleFunc(PatternApiQuery, self.handleHttpApiQuery)
	sv.HandleFunc(PatternApiQuestion, self.handleHttpApiQuestion)
	sv.HandleFunc(PatternApiAnswer, self.handleHttpApiAnswer)
	sv.HandleFunc(PatternApiList, self.handleHttpApiList)
	sv.HandleFunc(PatternViewAnswer, self.handleHttpViewAnswer)
	sv.HandleFunc(PatternViewList, self.handleHttpViewList)
	err := http.ListenAndServe(self.HttpAddr, sv)
	if err != nil {
		log.Fatalf("%v", err)
	}
}

type HttpAskReqData struct {
	Class    string
	Title    string
	Question string
}

type HttpAskRspData struct {
	Id string
}

func (self *QaServer) handleHttpViewQuestionNotFound(w http.ResponseWriter, r *http.Request) {
	//http.Redirect(w, r, PatternViewList, 404)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(404)
	html, _ := ioutil.ReadFile("qaroot/view/404.html")
	w.Write(html)
	//http.ServeFile(w, r, "qaroot/view/404.html")
}

func (self *QaServer) handleHttpApiAsk(w http.ResponseWriter, r *http.Request) {
	var payload HttpReq
	raw, _ := ioutil.ReadAll(r.Body)
	log.Printf("%s|%s", r.RequestURI, string(raw))
	err := json.Unmarshal(raw, &payload)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	var reqData HttpAskReqData
	err = mapstructure.Decode(payload.Data, &reqData)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	imgData, err := base64.StdEncoding.DecodeString(reqData.Question)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	id, _, _ := self.idWorker.NextId()
	v := &QuestionAndAnwser{reqData.Title, imgData, time.Now(), nil}
	self.db.Store(id, v)
	idStr := strconv.FormatInt(id, 16)
	rspData := &HttpAskRspData{idStr}
	responseData(w, 1, rspData)
}

type HttpQueryReqData struct {
	//Id string
}

type HttpQueryRspData struct {
	Answer string
}

func (self *QaServer) handleHttpApiQuery(w http.ResponseWriter, r *http.Request) {
	log.Printf(r.RequestURI)
	uri := r.RequestURI
	n := strings.Index(uri, PatternApiQuery)
	if n < 0 {
		responseError(w, 1, CodeQuestionNotFound, "question not found")
		//http.NotFound(w, r)
		return
	}
	idStr := uri[n+PatternLenApiQuery:]

	id, err := strconv.ParseInt(idStr, 16, 64)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	v_, ok := self.db.Load(id)
	if !ok {
		responseError(w, 1, CodeQuestionNotFound, "question not found")
		//http.NotFound(w, r)
		return
	}

	v := v_.(*QuestionAndAnwser)
	if v.Answer == nil {
		responseError(w, 1, CodeNoAnswer, "no answer")
		return
	}

	answer := v.Answer.(string)
	rspData := &HttpQueryRspData{answer}
	responseData(w, 1, rspData)
}

type HttpQuestionReqData struct {
	//Id     string
}

type HttpQuestionRspData struct {
}

func (self *QaServer) handleHttpApiQuestion(w http.ResponseWriter, r *http.Request) {
	log.Printf(r.RequestURI)
	uri := r.RequestURI
	n := strings.Index(uri, PatternApiQuestion)
	if n < 0 {
		http.NotFound(w, r)
		return
	}
	idStr := uri[n+PatternLenApiQuestion:]

	id, err := strconv.ParseInt(idStr, 16, 64)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	v_, ok := self.db.Load(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	v := v_.(*QuestionAndAnwser)
	if v.Question == nil {
		responseError(w, 1, CodeAnswered, "answered")
		return
	}

	imgData := v.Question.([]byte)
	w.Header().Add("content-type", "image/jpg")
	w.Write(imgData)
}

type HttpAnswerReqData struct {
	//Id     string
	Answer string
}

type HttpAnswerRspData struct {
}

func (self *QaServer) handleHttpApiAnswer(w http.ResponseWriter, r *http.Request) {
	uri := r.RequestURI
	n := strings.Index(uri, PatternApiAnswer)
	if n < 0 {
		responseError(w, 1, CodeQuestionNotFound, "question not found")
		//http.NotFound(w, r)
		return
	}
	idStr := uri[n+PatternLenApiAnswer:]

	var payload HttpReq
	raw, _ := ioutil.ReadAll(r.Body)
	log.Printf("%s|%s", r.RequestURI, string(raw))
	err := json.Unmarshal(raw, &payload)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	var reqData HttpAnswerReqData
	err = mapstructure.Decode(payload.Data, &reqData)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	log.Printf("Answer(%s): %s", idStr, reqData.Answer)
	id, err := strconv.ParseInt(idStr, 16, 64)
	if err != nil {
		log.Printf(err.Error())
		responseError(w, 1, CodeError, err.Error())
		return
	}

	v_, ok := self.db.Load(id)
	if !ok {
		responseError(w, 1, CodeQuestionNotFound, "question not found")
		//http.NotFound(w, r)
		return
	}
	v := v_.(*QuestionAndAnwser)

	v2 := &QuestionAndAnwser{v.Title, nil, v.Time, reqData.Answer}
	self.db.Store(id, v2)

	rspData := &HttpAnswerRspData{}
	responseData(w, 1, rspData)
}

type QuestionStatus struct {
	Id     string
	Time   int64
	Status string
}
type HttpListRspData struct {
	List []QuestionStatus
}

func (self *QaServer) handleHttpApiList(w http.ResponseWriter, r *http.Request) {
	log.Printf(r.RequestURI)

	lst := make([]QuestionStatus, 0)
	self.db.Range(func(k_, v_ interface{}) bool {
		k := k_.(int64)
		v := v_.(*QuestionAndAnwser)
		idStr := strconv.FormatInt(k, 16)
		var status string
		if v.Answer != nil {
			status = "1"
		} else {
			status = "0"
		}
		lst = append(lst, QuestionStatus{idStr, v.Time.Unix(), status})
		return true
	})

	rspData := &HttpListRspData{lst}
	responseData(w, 1, rspData)
}

type AnswerView struct {
}

func (self *QaServer) handleHttpViewAnswer(w http.ResponseWriter, r *http.Request) {
	log.Printf(r.RequestURI)
	uri := r.RequestURI
	n := strings.Index(uri, PatternViewAnswer)
	if n < 0 {
		//http.NotFound(w, r)
		self.handleHttpViewQuestionNotFound(w, r)
		return
	}
	idStr := uri[n+PatternLenViewAnswer:]

	id, err := strconv.ParseInt(idStr, 16, 64)
	if err != nil {
		log.Printf(err.Error())
		self.handleHttpViewQuestionNotFound(w, r)
		return
	}

	_, ok := self.db.Load(id)
	if !ok {
		//http.NotFound(w, r)
		self.handleHttpViewQuestionNotFound(w, r)
		return
	}

	http.ServeFile(w, r, "qaroot/view/answer.html")
}

func (self *QaServer) handleHttpViewList(w http.ResponseWriter, r *http.Request) {
	log.Printf(r.RequestURI)
	http.ServeFile(w, r, "qaroot/view/list.html")
}

func responseError(w http.ResponseWriter, ver int, errcode HttpRspCode, errmsg string) {
	w.Header().Add("content-type", "application/json; charset=utf-8")
	rsp := HttpRsp{
		ver,
		errcode,
		errmsg,
		EMPTY_HTTP_RSP_DATA,
	}
	jsStr, err := json.Marshal(rsp)
	if err != nil {
		log.Printf(err.Error())
		return
	}
	w.Write(jsStr)
}

func responseData(w http.ResponseWriter, ver int, data interface{}) {
	w.Header().Add("content-type", "application/json; charset=utf-8")
	rsp := HttpRsp{
		ver,
		CodeSuccess,
		"",
		data,
	}
	jsStr, err := json.Marshal(rsp)
	if err != nil {
		rsp := HttpRsp{
			ver,
			CodeError,
			err.Error(),
			EMPTY_HTTP_RSP_DATA,
		}
		jsStr, err = json.Marshal(rsp)
		if err != nil {
			log.Printf(err.Error())
			return
		}
	}
	w.Write(jsStr)
}

func (self *QaServer) checkExpired() {
	lastCheck := time.Now()
	var checkDelta time.Duration
	if self.ExpiredAnswered < self.ExpiredUnanswered {
		checkDelta = self.ExpiredAnswered
	} else {
		checkDelta = self.ExpiredUnanswered
	}

	for {
		now := time.Now()
		delta := checkDelta - now.Sub(lastCheck)
		if delta <= 0 {
			lastCheck = now
			expiredList := make([]interface{}, 0)
			self.db.Range(func(k_, v_ interface{}) bool {
				v := v_.(*QuestionAndAnwser)
				if (v.Answer != nil && now.Sub(v.Time) >= self.ExpiredAnswered) || (v.Answer == nil && now.Sub(v.Time) >= self.ExpiredUnanswered) {
					expiredList = append(expiredList, k_)
				}
				return true
			})
			for _, expired := range expiredList {
				self.db.Delete(expired)
			}
		} else {
			time.Sleep(delta)
		}
	}
}
