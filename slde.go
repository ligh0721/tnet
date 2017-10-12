package tnet

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"time"
)

const (
	SLDE_STX         byte = 2
	SLDE_ETX         byte = 3
	SLDE_CUSTOM_SIZE int  = 4 // TODO: add custom size
	SLDE_LENGTH_SIZE int  = 4
	SLDE_HEADER_SIZE int  = SLDE_CUSTOM_SIZE + SLDE_LENGTH_SIZE + 1

	xor_encrypt_seed int64 = 776103
)

type Slde struct {
	writebuf *bytes.Buffer
	length   int

	// custom fields
	rid uint32
}

func xorEncrypt(data []byte, seed int64) (ret []byte) {
	ret = make([]byte, len(data))
	rnd := rand.New(rand.NewSource(seed))
	for i, v := range data {
		ret[i] = v ^ byte(rnd.Intn(256))
	}
	return ret
}

func encrypt(data []byte) (ret []byte) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(data)
	w.Close()
	ret = xorEncrypt(b.Bytes(), xor_encrypt_seed)
	return ret
}

func decrypt(data []byte) (ret []byte, err error) {
	data = xorEncrypt(data, xor_encrypt_seed)
	b := bytes.NewBuffer(data)
	r, err := zlib.NewReader(b)
	if err != nil {
		return nil, err
	}
	ret, err = ioutil.ReadAll(r)
	r.Close()
	return ret, nil
}

func (self *Slde) Write(data []byte) (left int, err error) {
	self.writebuf.Write(data)

	if self.length < 0 {
		if self.writebuf.Len() < SLDE_HEADER_SIZE {
			// header not enough
			return SLDE_HEADER_SIZE - self.writebuf.Len(), nil
		}

		// header enough
		var stx byte
		binary.Read(self.writebuf, binary.BigEndian, &stx)
		if stx != SLDE_STX {
			return -1, errors.New("field stx err")
		}

		// TODO: add custom field
		binary.Read(self.writebuf, binary.BigEndian, &self.rid)
		log.Printf("decode slde.rid: %04X", self.rid)

		var length int32
		binary.Read(self.writebuf, binary.BigEndian, &length)
		if length < 0 {
			return -1, errors.New("field length err")
		}
		self.length = int(length)
		//log.Println("decode slde.length:", self.length)
	}

	left = self.length + 1 - self.writebuf.Len()
	if left > 0 {
		return left, nil
	}

	// write finished
	etx := self.writebuf.Bytes()[self.length]
	if etx != SLDE_ETX {
		return -1, errors.New("field etx err")
	}

	return 0, nil
}

func (self *Slde) Decode() (ret []byte, err error) {
	if self.length < 0 || self.writebuf.Len() != self.length+1 {
		return nil, errors.New(fmt.Sprintf("data format err, length field(%d), real data field length(%d), raw data: [% x]", self.length, self.writebuf.Len()-1, self.writebuf.Bytes()))
	}

	ret = self.writebuf.Bytes()[:self.length]
	ret, err = decrypt(ret)
	return ret, err
}

func (self *Slde) DecodeAndReset() (ret []byte, err error) {
	ret, err = self.Decode()
	if err == nil {
		self.Reset()
	}
	return ret, err
}

func (self *Slde) Encode(data []byte) (ret []byte, err error) {
	data = encrypt(data)
	self.length = len(data)
	//log.Println("encode slde.length:", self.length)
	self.writebuf.Reset()
	binary.Write(self.writebuf, binary.BigEndian, SLDE_STX)

	// TODO: add custom fields
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	self.rid = rnd.Uint32()
	log.Printf("encode slde.rid: %04X", self.rid)
	binary.Write(self.writebuf, binary.BigEndian, self.rid)

	binary.Write(self.writebuf, binary.BigEndian, int32(self.length))
	self.writebuf.Write(data)
	binary.Write(self.writebuf, binary.BigEndian, SLDE_ETX)
	return self.writebuf.Bytes(), nil
}

func (self *Slde) Bytes() (ret []byte) {
	return self.writebuf.Bytes()
}

func (self *Slde) Reset() {
	self.writebuf.Reset()
	self.length = -1

	// TODO: reset custom fields
	self.rid = 0
}

func NewSlde() (obj *Slde) {
	obj = new(Slde)
	obj.writebuf = bytes.NewBuffer([]byte{})
	obj.length = -1
	return obj
}

func NewSldeWithData(data []byte) (obj *Slde) {
	obj = new(Slde)
	obj.writebuf = bytes.NewBuffer([]byte{})
	obj.Encode(data)
	return obj
}
