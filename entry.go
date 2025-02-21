package jlog

import (
	"math"
	"strconv"
	"time"
)

type entry struct {
	s     severity
	buf   *buffer
}

func (e *entry) Msg(msg string) {
	e.buf.Write(str2bytes(`,"msg":"`))
	e.buf.Write(str2bytes(msg))
	e.buf.writeByte('"')
	e.buf.writeByte('}')
	e.buf.writeByte('\n')
	loggers[e.s].output(e.buf.bytes())
	freeEntry(e)
	return
}

func (e *entry) Str(key string, val string) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(val))
	e.buf.writeByte('"')
	return e
}

func (e *entry) Bytes(key string, val []byte) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	e.buf.writeByte('"')
	e.buf.Write(val)
	e.buf.writeByte('"')
	return e
}

func (e *entry) Byte(key string, val byte) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	e.buf.writeByte(val)
	return e
}

func (e *entry) Bool(key string, val bool) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	if val {
		e.buf.Write(str2bytes("true"))
	} else {
		e.buf.Write(str2bytes("false"))
	}
	return e
}

func intStringLen(n int64) int {
	if n == 0 {
		return 1 // 0 的字符串长度为 1
	}

	length := 0
	if n < 0 {
		length = 1 // 负数符号占 1 位
		n = -n     // 转为正数计算
	}

	// 通过除法统计位数
	for n > 0 {
		length++
		n /= 10
	}
	return length
}

func (e *entry) Int(key string, val int) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')

	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendInt(e.buf.bytes(), int64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Int64(key string, val int64) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')


	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendInt(e.buf.bytes(), val, 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Int32(key string, val int32) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendInt(e.buf.bytes(), int64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Int16(key string, val int16) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendInt(e.buf.bytes(), int64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Int8(key string, val int8) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendInt(e.buf.bytes(), int64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Uint(key string, val uint) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')

	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendUint(e.buf.bytes(),  uint64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Uint64(key string, val uint64) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendUint(e.buf.bytes(),  uint64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Uint32(key string, val uint32) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendUint(e.buf.bytes(),  uint64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Uint16(key string, val uint16) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendUint(e.buf.bytes(),  uint64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Uint8(key string, val uint8) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	buf := e.buf.bytes()
	len1 := len(buf)
	buf = strconv.AppendUint(e.buf.bytes(),  uint64(val), 10)
	e.buf.roff += len(buf) - len1
	return e
}

func (e *entry) Float64(key string, val float64) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')

	switch {
	case math.IsNaN(val):
		e.buf.Write(str2bytes(`"NaN"`))
	case math.IsInf(val, 1):
		e.buf.Write(str2bytes(`"+Inf"`))
	case math.IsInf(val, -1):
		e.buf.Write(str2bytes(`"-Inf"`))
	default:
		buf := e.buf.bytes()
		len1 := len(buf)
		buf = strconv.AppendFloat(e.buf.bytes(), val, 'f', -1, 64)
		e.buf.roff += len(buf) - len1
	}
	return e
}

func (e *entry) Float32(key string, val float32) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	vv := float64(val)
	switch {
	case math.IsNaN(vv):
		e.buf.Write(str2bytes(`"NaN"`))
	case math.IsInf(vv, 1):
		e.buf.Write(str2bytes(`"+Inf"`))
	case math.IsInf(vv, -1):
		e.buf.Write(str2bytes(`"-Inf"`))
	default:
		buf := e.buf.bytes()
		len1 := len(buf)
		buf = strconv.AppendFloat(e.buf.bytes(), vv, 'f', -1, 32)
		e.buf.roff += len(buf) - len1
	}
	return e
}

func (e *entry) time(val time.Time) *entry {
	if e == nil {
		return nil
	}
	e.buf.Write(str2bytes(`"time":"`))
	e.buf.writeTime(val)
	e.buf.writeByte('"')
	return e
}

func (e *entry) Time(key string, val time.Time) *entry {
	if e == nil {
		return nil
	}
	e.buf.writeByte(',')
	e.buf.writeByte('"')
	e.buf.Write(str2bytes(key))
	e.buf.writeByte('"')
	e.buf.writeByte(':')
	e.buf.writeByte('"')

	e.buf.writeTime(val)
	e.buf.writeByte('"')
	return e
}

func (e *entry) ReqId(val string) *entry {
	if e == nil {
		return nil
	}
	e.buf.Write(str2bytes(`,"reqId":"`))
	e.buf.Write(str2bytes(val))
	e.buf.writeByte('"')
	return e
}

func (e *entry) level(val string) *entry {
	if e == nil {
		return nil
	}
	e.buf.Write(str2bytes(`,"level":"`))
	e.buf.Write(str2bytes(val))
	e.buf.writeByte('"')
	return e
}
