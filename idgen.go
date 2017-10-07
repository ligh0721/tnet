package tnet

// +---------------+----------------+----------------+
// |timestamp(ms)42  | worker id(10) | sequence(12)	 |
// +---------------+----------------+----------------+

import (
	"errors"
	"sync"
	"time"
)

const (
	CEpoch         = 1506666666
	CWorkerIdBits  = 10 // Num of WorkerId Bits
	CSenquenceBits = 12 // Num of Sequence Bits

	CWorkerIdShift  = 12
	CTimeStampShift = 22

	CSequenceMask = 0xfff // equal as getSequenceMask()
	CMaxWorker    = 0x3ff // equal as getMaxWorkerId()
)

// IdWorker Struct
type IdWorker struct {
	workerId      int64
	lastTimeStamp int64
	sequence      int64
	maxWorkerId   int64
	lock          *sync.Mutex
}

// NewIdWorker Func: Generate NewIdWorker with Given workerid
func NewIdWorker(workerid int64) (obj *IdWorker, err error) {
	obj = new(IdWorker)

	obj.maxWorkerId = getMaxWorkerId()

	if workerid > obj.maxWorkerId || workerid < 0 {
		return nil, errors.New("worker not fit")
	}
	obj.workerId = workerid
	obj.lastTimeStamp = -1
	obj.sequence = 0
	obj.lock = new(sync.Mutex)
	return obj, nil
}

func getMaxWorkerId() int64 {
	return -1 ^ -1<<CWorkerIdBits
}

func getSequenceMask() int64 {
	return -1 ^ -1<<CSenquenceBits
}

// return in ms
func (self *IdWorker) timeGen() int64 {
	return time.Now().UnixNano() / 1000 / 1000
}

// NewId Func: Generate next id
func (self *IdWorker) NextId() (ret int64, time int64, err error) {
	self.lock.Lock()
	ts := self.timeGen()
	if ts > self.lastTimeStamp {
		self.sequence = 0
	} else if ts == self.lastTimeStamp {
		self.sequence = (self.sequence + 1) & CSequenceMask
		if self.sequence == 0 {
			var nextTs int64
			for {
				nextTs = self.timeGen()
				if nextTs > ts {
					break
				}
			}
			ts = nextTs
		}
	} else {
		self.lock.Unlock()
		err = errors.New("clock moved backwards, Refuse gen id")
		return 0, ts, err
	}

	self.lastTimeStamp = ts
	self.lock.Unlock()
	ret = (ts-CEpoch)<<CTimeStampShift | self.workerId<<CWorkerIdShift | self.sequence
	return ret, ts, nil
}

// ParseId Func: reverse uid to timestamp, workid, seq
func ParseId(id int64) (t time.Time, ts int64, workerId int64, seq int64) {
	seq = id & CSequenceMask
	workerId = (id >> CWorkerIdShift) & CMaxWorker
	ts = (id >> CTimeStampShift) + CEpoch
	t = time.Unix(ts/1000, (ts%1000)*1e6)
	return t, ts, workerId, seq
}
