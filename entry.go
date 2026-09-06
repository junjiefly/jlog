package jlog

import (
	"math"
	"strconv"
	"time"
)

type entry struct {
	logger       *iLog
	buf          *buffer
	reserved     map[string]struct{}
	baseReserved bool
}

// minFieldHeadroom keeps slack for the closing part of a JSON record so an
// overlong entry still serializes into parseable JSON.
const minFieldHeadroom = 96

func (e *entry) canAppend() bool {
	return e != nil && e.buf != nil && e.buf.remaining() >= minFieldHeadroom
}

func (e *entry) canAppendKey(key string) bool {
	if !e.canAppend() {
		return false
	}
	if e.baseReserved {
		switch key {
		case "time", "msg", "level":
			return false
		}
	}
	if e.reserved == nil {
		return true
	}
	_, reserved := e.reserved[key]
	return !reserved
}

func (e *entry) Msg(msg string) {
	if e == nil || e.buf == nil {
		return
	}
	switch {
	case e.buf.remaining() < 2:
		// Only the final newline slot is left. Reuse the opening brace to emit
		// a valid empty object instead of an unterminated partial record.
		e.buf.roff = 1
		e.buf.writeByte('}')
	case e.buf.remaining() >= 11:
		e.buf.Write(str2bytes(`,"msg":`))
		e.buf.writeJSONString(msg)
		e.buf.writeByte('}')
	case e.buf.remaining() >= 1:
		e.buf.writeByte('}')
	}
	e.buf.writeByte('\n')
	e.logger.output(e.buf.bytes())
	freeEntry(e)
	return
}

func (e *entry) Str(key string, val string) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	e.buf.writeJSONString(val)
	return e
}

func (e *entry) Bytes(key string, val []byte) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	e.buf.writeJSONString(bytes2str(val))
	return e
}

// Err appends the conventional "error" field. A nil error is omitted.
func (e *entry) Err(err error) *entry {
	if err == nil || !e.canAppendKey("error") {
		return e
	}
	e.buf.writeJSONKey("error")
	e.buf.writeJSONString(err.Error())
	return e
}

func (e *entry) Byte(key string, val byte) *entry {
	return e.appendUintVal(key, uint64(val))
}

func (e *entry) Bool(key string, val bool) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	if val {
		e.buf.Write(str2bytes("true"))
	} else {
		e.buf.Write(str2bytes("false"))
	}
	return e
}

func (e *entry) appendIntVal(key string, val int64) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	var tmp [24]byte
	e.buf.writeRaw(strconv.AppendInt(tmp[:0], val, 10))
	return e
}

func (e *entry) appendUintVal(key string, val uint64) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	var tmp [24]byte
	e.buf.writeRaw(strconv.AppendUint(tmp[:0], val, 10))
	return e
}

func (e *entry) Int(key string, val int) *entry {
	return e.appendIntVal(key, int64(val))
}

func (e *entry) Int64(key string, val int64) *entry {
	return e.appendIntVal(key, val)
}

func (e *entry) Int32(key string, val int32) *entry {
	return e.appendIntVal(key, int64(val))
}

func (e *entry) Int16(key string, val int16) *entry {
	return e.appendIntVal(key, int64(val))
}

func (e *entry) Int8(key string, val int8) *entry {
	return e.appendIntVal(key, int64(val))
}

func (e *entry) Uint(key string, val uint) *entry {
	return e.appendUintVal(key, uint64(val))
}

func (e *entry) Uint64(key string, val uint64) *entry {
	return e.appendUintVal(key, val)
}

func (e *entry) Uint32(key string, val uint32) *entry {
	return e.appendUintVal(key, uint64(val))
}

func (e *entry) Uint16(key string, val uint16) *entry {
	return e.appendUintVal(key, uint64(val))
}

func (e *entry) Uint8(key string, val uint8) *entry {
	return e.appendUintVal(key, uint64(val))
}

func (e *entry) Float64(key string, val float64) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	switch {
	case math.IsNaN(val):
		e.buf.writeJSONString("NaN")
	case math.IsInf(val, 1):
		e.buf.writeJSONString("+Inf")
	case math.IsInf(val, -1):
		e.buf.writeJSONString("-Inf")
	default:
		var tmp [40]byte
		e.buf.writeRaw(strconv.AppendFloat(tmp[:0], val, 'f', -1, 64))
	}
	return e
}

func (e *entry) Float32(key string, val float32) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	vv := float64(val)
	switch {
	case math.IsNaN(vv):
		e.buf.writeJSONString("NaN")
	case math.IsInf(vv, 1):
		e.buf.writeJSONString("+Inf")
	case math.IsInf(vv, -1):
		e.buf.writeJSONString("-Inf")
	default:
		var tmp [40]byte
		e.buf.writeRaw(strconv.AppendFloat(tmp[:0], vv, 'f', -1, 32))
	}
	return e
}

func (e *entry) time(val time.Time) *entry {
	// Internal base time is written before caller fields. It intentionally
	// bypasses reserved keys so custom logs can reserve "time" for callers.
	if !e.canAppend() {
		return nil
	}
	e.buf.Write(str2bytes(`"time":`))
	e.buf.writeJSONTimeString(val)
	return e
}

func (e *entry) Time(key string, val time.Time) *entry {
	if !e.canAppendKey(key) {
		return e
	}
	e.buf.writeJSONKey(key)
	e.buf.writeJSONTimeString(val)
	return e
}

func (e *entry) ReqId(val string) *entry {
	if !e.canAppendKey("reqId") {
		return e
	}
	e.buf.writeJSONKey("reqId")
	e.buf.writeJSONString(val)
	return e
}

func (e *entry) level(val string) *entry {
	if !e.canAppendKey("level") {
		return e
	}
	e.buf.writeJSONKey("level")
	e.buf.writeJSONString(val)
	return e
}
