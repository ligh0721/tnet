package tcenter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"git.tutils.com/tutils/tnet"
	"github.com/golang/protobuf/proto"
)

type PbSeri struct {
}

func NewPbSeri() (obj *PbSeri) {
	obj = &PbSeri{}
	return obj
}

func (self *PbSeri) Marshal(obj interface{}) (ret []byte, err error) {
	pb := obj.(proto.Message)
	return proto.Marshal(pb)
}

func (self *PbSeri) Unmarshal(data []byte, pobj interface{}) (err error) {
	pb := pobj.(proto.Message)
	return proto.Unmarshal(data, pb)
}

func encodeTCenterData(cmd string, seri tnet.Serializer, obj interface{}) (ret []byte, err error) {
	buf := bytes.NewBuffer([]byte{})

	cmdLen := uint8(len(cmd))
	binary.Write(buf, binary.BigEndian, cmdLen)

	cmdBytes := []byte(cmd)
	binary.Write(buf, binary.BigEndian, cmdBytes)

	data, err := seri.Marshal(obj)
	if err != nil {
		return nil, err
	}
	binary.Write(buf, binary.BigEndian, data)

	return tnet.EncodeToSldeDataFromBytes(buf.Bytes())
}

func decodeTCenterData(cmd string, seri tnet.Serializer, data []byte, pobj interface{}) (err error) {
	buf := bytes.NewBuffer(data)

	var cmdLen uint8
	binary.Read(buf, binary.BigEndian, &cmdLen)

	cmdBytes := make([]byte, cmdLen)
	binary.Read(buf, binary.BigEndian, &cmdBytes)
	cmdStr := string(cmdBytes)
	if cmdStr != cmd {
		return errors.New("cmd(%s) err, cmd(%s) expected")
	}

	return seri.Unmarshal(buf.Bytes(), pobj)
}
