package jlog

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
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
	return atomic.LoadInt64(&bufTotal)
}

var bufferPool sync.Pool

var entryPool sync.Pool

func newEntry() *entry {
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
		e.buf = nil
	}
	e.reserved = nil
	e.baseReserved = false
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
				buf: make([]byte, maxBufSize),
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
	if fb.buf == nil {
		return 0, errors.New("buffer empty")
	}
	// Reserve the final byte for line termination. This guarantees that text
	// records remain one-per-line even when a caller writes up to capacity.
	extra := maxBufSize - fb.roff - 1
	if extra <= 0 {
		return 0, nil
	}
	if len(buf) > extra {
		// Truncate into our own buffer; never write through to the caller's
		// slice and never inject bytes that were not in the input.
		buf = buf[:extra]
	}
	m := copy(fb.buf[fb.roff:], buf)
	fb.roff += m
	return m, nil
}

func (fb *buffer) writeByte(b byte) (n int, err error) {
	if fb.buf == nil || fb.roff >= maxBufSize {
		return 0, nil
	}
	// Regular bytes keep one slot free for the terminating newline; the
	// newline itself may use that final slot.
	if b != lineBreak && fb.roff >= maxBufSize-1 {
		return 0, nil
	}
	fb.buf[fb.roff] = b
	fb.roff++
	return 1, nil
}

func (fb *buffer) writeRaw(b []byte) {
	extra := maxBufSize - fb.roff
	if extra <= 0 {
		return
	}
	if headroom := extra - 4; headroom > 0 {
		if len(b) > headroom {
			// Always leave room so a JSON record can still be closed.
			b = b[:headroom]
		}
		fb.roff += copy(fb.buf[fb.roff:], b)
	}
}

func (fb *buffer) remaining() int {
	if fb.buf == nil {
		return 0
	}
	return len(fb.buf) - fb.roff
}

const jsonStrHeadroom = 16

const hexDigits = "0123456789abcdef"

func (fb *buffer) writeJSONString(s string) {
	fb.writeByte('"')
	i := 0
	for i < len(s) {
		rem := fb.remaining()
		if rem < jsonStrHeadroom {
			break
		}
		limit := i + rem - jsonStrHeadroom
		if limit > len(s) {
			limit = len(s)
		}
		j := i
		for j < limit {
			c := s[j]
			if c >= utf8.RuneSelf {
				break
			}
			if c < 0x20 || c == '"' || c == '\\' {
				break
			}
			j++
		}
		if j > i {
			fb.writeRaw(str2bytes(s[i:j]))
			i = j
			continue
		}
		if i >= limit {
			break
		}
		c := s[i]
		if c >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				fb.writeRaw(str2bytes(`\ufffd`))
				i++
				continue
			}
			if r == 0x2028 || r == 0x2029 {
				fb.writeRaw(str2bytes(`\u202`))
				if r == 0x2028 {
					fb.writeByte('8')
				} else {
					fb.writeByte('9')
				}
				i += size
				continue
			}
			fb.writeRaw(str2bytes(s[i : i+size]))
			i += size
			continue
		}
		fb.writeByte('\\')
		switch c {
		case '"', '\\':
			fb.writeByte(c)
		case '\n':
			fb.writeByte('n')
		case '\r':
			fb.writeByte('r')
		case '\t':
			fb.writeByte('t')
		case '\b':
			fb.writeByte('b')
		case '\f':
			fb.writeByte('f')
		default:
			fb.writeRaw(str2bytes("u00"))
			fb.writeByte(hexDigits[c>>4])
			fb.writeByte(hexDigits[c&0x0f])
		}
		i++
	}
	fb.writeByte('"')
}

func (fb *buffer) writeJSONKey(key string) {
	fb.writeByte(',')
	fb.writeJSONString(key)
	fb.writeByte(':')
}

func (fb *buffer) writeJSONStringBytes(s []byte) {
	fb.writeByte('"')
	i := 0
	for i < len(s) {
		rem := fb.remaining()
		if rem < jsonStrHeadroom {
			break
		}
		limit := i + rem - jsonStrHeadroom
		if limit > len(s) {
			limit = len(s)
		}
		j := i
		for j < limit {
			c := s[j]
			if c >= utf8.RuneSelf {
				break
			}
			if c < 0x20 || c == '"' || c == '\\' {
				break
			}
			j++
		}
		if j > i {
			fb.writeRaw(s[i:j])
			i = j
			continue
		}
		if i >= limit {
			break
		}
		c := s[i]
		if c >= utf8.RuneSelf {
			r, size := utf8.DecodeRune(s[i:])
			if r == utf8.RuneError && size == 1 {
				fb.writeRaw([]byte(`\ufffd`))
				i++
				continue
			}
			if r == 0x2028 || r == 0x2029 {
				fb.writeRaw([]byte(`\u202`))
				if r == 0x2028 {
					fb.writeByte('8')
				} else {
					fb.writeByte('9')
				}
				i += size
				continue
			}
			fb.writeRaw(s[i : i+size])
			i += size
			continue
		}
		fb.writeByte('\\')
		switch c {
		case '"', '\\':
			fb.writeByte(c)
		case '\n':
			fb.writeByte('n')
		case '\r':
			fb.writeByte('r')
		case '\t':
			fb.writeByte('t')
		case '\b':
			fb.writeByte('b')
		case '\f':
			fb.writeByte('f')
		default:
			fb.writeRaw([]byte("u00"))
			fb.writeByte(hexDigits[c>>4])
			fb.writeByte(hexDigits[c&0x0f])
		}
		i++
	}
	fb.writeByte('"')
}

func (fb *buffer) writeJSONTimeString(t time.Time) {
	var tmp [64]byte
	formatted := t.AppendFormat(tmp[:0], currentTimeFormat())
	fb.writeJSONStringBytes(formatted)
}

func (fb *buffer) bytes() []byte {
	if fb.buf == nil {
		return nil
	} else {
		return fb.buf[fb.woff:fb.roff]
	}
}

func (fb *buffer) writeTime(t time.Time) {
	if fb.buf == nil {
		return
	}
	var tmp [64]byte
	formatted := t.AppendFormat(tmp[:0], currentTimeFormat())
	for i := 0; i < len(formatted); i++ {
		if formatted[i] == '\n' || formatted[i] == '\r' {
			writeTextField(fb, bytes2str(formatted))
			return
		}
	}
	fb.writeRaw(formatted)
}
