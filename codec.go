package tnet

import (
    "bytes"
    "compress/zlib"
    "io/ioutil"
    "math/rand"
)

type Codec interface {
    Encrypt(data []byte) (ret []byte)
    Decrypt(data []byte) (ret []byte, err error)
}

func xorEncrypt(data []byte, seed int64) (ret []byte) {
    ret = make([]byte, len(data))
    rnd := rand.New(rand.NewSource(seed))
    for i, v := range data {
        ret[i] = v ^ byte(rnd.Intn(256))
    }
    return ret
}

type XorCodec struct {
    seed int64
}

func NewXorCodec(seed int64) (obj Codec) {
    obj = &XorCodec{seed}
    return obj
}

func (self *XorCodec) Encrypt(data []byte) (ret []byte) {
    return xorEncrypt(data, self.seed)
}

func (self *XorCodec) Decrypt(data []byte) (ret []byte, err error) {
    return xorEncrypt(data, self.seed), nil
}

func zlibXorEncrypt(data []byte, seed int64) (ret []byte) {
    var b bytes.Buffer
    w := zlib.NewWriter(&b)
    w.Write(data)
    w.Close()
    ret = xorEncrypt(b.Bytes(), seed)
    return ret
}

func zlibXorDecrypt(data []byte, seed int64) (ret []byte, err error) {
    data = xorEncrypt(data, seed)
    b := bytes.NewBuffer(data)
    r, err := zlib.NewReader(b)
    if err != nil {
        return nil, err
    }
    ret, err = ioutil.ReadAll(r)
    r.Close()
    return ret, nil
}

type ZlibCodec struct {
}

func NewZlibCodec(seed int64) (obj Codec) {
    obj = &XorCodec{seed}
    return obj
}

func (self *ZlibCodec) Encrypt(data []byte) (ret []byte) {
    var b bytes.Buffer
    w := zlib.NewWriter(&b)
    w.Write(data)
    w.Close()
    return b.Bytes()
}

func (self *ZlibCodec) Decrypt(data []byte) (ret []byte, err error) {
    b := bytes.NewBuffer(data)
    r, err := zlib.NewReader(b)
    if err != nil {
        return nil, err
    }
    ret, err = ioutil.ReadAll(r)
    r.Close()
    return ret, nil
}

type ZlibXorCodec struct {
    ZlibCodec
    XorCodec
}

func NewZlibXorCodec(seed int64) (obj Codec) {
    obj = &ZlibXorCodec{ZlibCodec{}, XorCodec{seed}}
    return obj
}

func (self *ZlibXorCodec) Encrypt(data []byte) (ret []byte) {
    return self.XorCodec.Encrypt(self.ZlibCodec.Encrypt(data))
}

func (self *ZlibXorCodec) Decrypt(data []byte) (ret []byte, err error) {
    data, err = self.XorCodec.Decrypt(data)
    return self.ZlibCodec.Decrypt(data)
}