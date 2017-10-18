package tnet

import "errors"

const (
    max_ch_size = 1024
)

type BytesChanIO struct {
    ch chan []byte
}

func NewBytesChanIO() (obj *BytesChanIO) {
    obj = &BytesChanIO{}
    obj.ch = make(chan []byte, max_ch_size)
    return obj
}

func (self *BytesChanIO) Read(p []byte) (n int, err error) {
    block, ok := <- self.ch
    if !ok {
        // closed chan
        return 0, errors.New("channel is already closed")
    }

    n = copy(p, block)
    return n, nil
}

func (self *BytesChanIO) Write(p []byte) (n int, err error) {
    n = len(p)
    block := make([]byte, n)
    n = copy(block, p)
    self.ch <- block
    return n, nil
}

func (self *BytesChanIO) Close() error {
    close(self.ch)
    return nil
}