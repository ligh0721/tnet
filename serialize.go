package tnet


type Serializer interface {
    Marshal(obj interface{}) (ret []byte, err error)
    Unmarshal(data []byte, robj interface{}) (err error)
}

type BufferSerializer interface {
    Marshal(obj interface{}) (ret []byte, err error)
    Unmarshal(data []byte, pobj interface{}) (err error)
}