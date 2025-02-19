package jlog

import (
	"errors"
	"sync"
	"sync/atomic"
)

//Simplified from fastBuffer

type buffer struct {
	buf  []byte //data bytes
	roff int    //how many bytes have been readed to fb from other user
	woff int    //how many bytes have been sended to other user
}

var bufTotal int64

var entryTotal int64

func stats() int64 {
	return  atomic.LoadInt64(&bufTotal)
}

var bufferPool sync.Pool

var entryPool sync.Pool

func newEntry()*entry{
	var e = entryPool.Get().(*entry)
	atomic.AddInt64(&entryTotal, 1)
	return e
}

func freeEntry(e *entry) {
	if e == nil {
		return
	}
	if e.buf != nil {
		freeBuffer(e.buf)
	}
	entryPool.Put(e)
	atomic.AddInt64(&entryTotal, -1)
}

func newBuffer() *buffer {
	var fb = bufferPool.Get().(*buffer)
	//fmt.Println("Cap:", cap(fb.buf), "len:", len(fb.buf))
	atomic.AddInt64(&bufTotal, 1)
	return fb
}

func freeBuffer(fb *buffer) {
	if fb == nil {
		return
	}
	fb.reset()
	bufferPool.Put(fb)
	atomic.AddInt64(&bufTotal, -1)
}

func init() {
	bufferPool = sync.Pool{
		New: func() interface{} {
			return &buffer{
				buf:  make([]byte,maxBufSize),
			}
		},
	}
	entryPool = sync.Pool{
		New: func() interface{} {
			return new(entry)
		},
	}
}

func (fb *buffer) reset() {
	fb.roff = 0
	fb.woff = 0
}

func (fb *buffer) Write(buf []byte) (n int, err error) {
	length := len(buf)
	extra := maxBufSize - fb.roff
	if extra <= 0 {
		return 0, nil
	}
	if extra < length {
		buf = buf[:extra]
		buf[extra-1] = '\n'
	}
	m := copy(fb.buf[fb.roff:], buf)
	fb.roff += m
	return m, nil
}

func (fb *buffer) writeByte(b byte) (n int, err error) {
	extra := maxBufSize - fb.roff
	if extra <= 0 {
		return 0, nil
	}
	fb.buf[fb.roff] = b
	fb.roff++
	return 1, nil
}

func (fb *buffer) getBuf() []byte {
	return fb.buf
}

func (fb *buffer) getReadOffset() int {
	if fb.buf == nil {
		return 0
	}
	return fb.roff
}

func (fb *buffer) bytes() []byte {
	if fb.buf == nil {
		return nil
	} else {
		return fb.buf[fb.woff:fb.roff]
	}
}

func (fb *buffer) resize(roff, woff int) error {
	if fb.buf == nil {
		return errors.New("buffer empty")
	}
	length := len(fb.buf)
	if roff > length || woff > length {
		return nil // io.ErrShortBuffer
	}
	if roff >= 0 {
		fb.roff = roff
	}
	if woff >= 0 {
		fb.woff = woff
	}
	return nil
}
