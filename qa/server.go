package qa

import (
    "net/http"
    "log"
    "encoding/json"
    "encoding/base64"
    "git.tutils.com/tutils/tnet"
    "io/ioutil"
    "strconv"
    "git.tutils.com/tutils/tnet/encoding/mapstructure"
    //"fmt"
    //"os"
    "sync"
    "strings"
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
    Title string
    Question interface{}
    Answer interface{}
}
type QaServer struct {
    HttpAddr string
    idWorker *tnet.IdWorker
    db *sync.Map
}

func NewQaServer() (obj *QaServer) {
    obj = new(QaServer)
    obj.idWorker, _ = tnet.NewIdWorker(0)
    obj.db = &sync.Map{}
    return obj
}

func (self *QaServer) Start() {
    //err := os.MkdirAll("VerifyCodeImg", 0755)  //生成多级目录
    //if err != nil {
    //    log.Fatalf("%v", err)
    //}
    self.serveHttp()
}

const (
    PatternQaAsk = "/qa/ask"
    PatternQaQuery = "/qa/query/"
    PatternQaQuestion = "/qa/question/"
    PatternQaAnswer = "/qa/answer/"
)

const (
    PatternLenQaAsk = len(PatternQaAsk)
    PatternLenQaQuery = len(PatternQaQuery)
    PatternLenQaQuestion = len(PatternQaQuestion)
    PatternLenQaAnswer = len(PatternQaAnswer)
)

func (self *QaServer) serveHttp() {
    sv := http.NewServeMux()
    sv.HandleFunc(PatternQaAsk, self.handleHttpAsk)
    sv.HandleFunc(PatternQaQuery, self.handleHttpQuery)
    sv.HandleFunc(PatternQaQuestion, self.handleHttpQuestion)
    sv.HandleFunc(PatternQaAnswer, self.handleHttpAnswer)
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

func (self *QaServer) handleHttpAsk(w http.ResponseWriter, r *http.Request) {
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
    v := &QuestionAndAnwser{reqData.Title, imgData, nil}
    self.db.Store(id, v)
    idStr := strconv.FormatInt(id, 16)
    //fileName := fmt.Sprintf("VerifyCodeImg/%s.jpg", idStr)
    //err = ioutil.WriteFile(fileName, imgData, 0644)
    //if err != nil {
    //    log.Printf(err.Error())
    //    responseError(w, 1, CodeError, err.Error())
    //    return
    //}

    rspData := &HttpAskRspData{idStr}
    responseData(w, 1, rspData)
}

type HttpQueryReqData struct {
    //Id string
}

type HttpQueryRspData struct {
    Answer string
}

func (self *QaServer) handleHttpQuery(w http.ResponseWriter, r *http.Request) {
    log.Printf(r.RequestURI)
    uri := r.RequestURI
    n := strings.Index(uri, PatternQaQuery)
    if n < 0 {
        http.NotFound(w, r)
        return
    }
    idStr := uri[n+PatternLenQaQuery:]

    //var payload HttpReq
    //raw, _ := ioutil.ReadAll(r.Body)
    //err := json.Unmarshal(raw, &payload)
    //if err != nil {
    //    log.Printf(err.Error())
    //    responseError(w, 1, CodeError, err.Error())
    //    return
    //}
    //
    //var reqData HttpQueryReqData
    //err = mapstructure.Decode(payload.Data, &reqData)
    //if err != nil {
    //    log.Printf(err.Error())
    //    responseError(w, 1, CodeError, err.Error())
    //    return
    //}

    id, err := strconv.ParseInt(idStr, 16, 64)
    if err != nil {
        log.Printf(err.Error())
        responseError(w, 1, CodeError, err.Error())
        return
    }

    v_, ok := self.db.Load(id)
    if !ok {
        responseError(w, 1, CodeQuestionNotFound, "question not found")
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

func (self *QaServer) handleHttpQuestion(w http.ResponseWriter, r *http.Request) {
    log.Printf(r.RequestURI)
    uri := r.RequestURI
    n := strings.Index(uri, PatternQaQuestion)
    if n < 0 {
        http.NotFound(w, r)
        return
    }
    idStr := uri[n+PatternLenQaQuestion:]

    //var payload HttpReq
    //raw, _ := ioutil.ReadAll(r.Body)
    //err := json.Unmarshal(raw, &payload)
    //if err != nil {
    //    log.Printf(err.Error())
    //    responseError(w, 1, CodeError, err.Error())
    //    return
    //}
    //
    //var reqData HttpQuestionReqData
    //err = mapstructure.Decode(payload.Data, &reqData)
    //if err != nil {
    //    log.Printf(err.Error())
    //    responseError(w, 1, CodeError, err.Error())
    //    return
    //}

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

func (self *QaServer) handleHttpAnswer(w http.ResponseWriter, r *http.Request) {
    uri := r.RequestURI
    n := strings.Index(uri, PatternQaAnswer)
    if n < 0 {
        http.NotFound(w, r)
        return
    }
    idStr := uri[n+PatternLenQaAnswer:]

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
        //responseError(w, 1, CodeQuestionNotFound, "question not found")
        http.NotFound(w, r)
        return
    }
    v := v_.(*QuestionAndAnwser)

    v2 := &QuestionAndAnwser{v.Title, nil, reqData.Answer}
    self.db.Store(id, v2)

    rspData := &HttpAnswerRspData{}
    responseData(w, 1, rspData)
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
        0,
        "",
        data,
    }
    jsStr, err := json.Marshal(rsp)
    if err != nil {
        rsp := HttpRsp{
            ver,
            -1,
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