package jlog

import (
	"fmt"
	lumberjack "github.com/junjiefly/lumberjack"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type syncWriter interface {
	Write([]byte) (int, error)
	Flush() error
	Close() error
	SetOutput(localWrite bool, writers []io.Writer)
}

type iLog struct {
	severity
	sync.RWMutex
	Writer syncWriter
}

func newLogger(s severity) iLog {
	return iLog{severity: s}
}

func (log *iLog) create() {
	logger := &lumberjack.Logger{
		Filename:   logCfg.logDir + "/" + logCfg.fileName + severityName[log.severity],
		MaxSize:    logCfg.maxSize * mb,
		MaxBackups: logCfg.maxBackups,
		MaxAge:     logCfg.maxAge,
		Compress:   logCfg.compress,
	}
	writers := logCfg.writers
	if logCfg.consoleOut {
		writers = append(writers, os.Stdout)
	}
	logger.SetOutput(logCfg.localWrite, writers)
	log.Writer = logger
}

func (log *iLog) write(p []byte) (n int, err error) {
	n, err = log.Writer.Write(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: log can't be done because of error: %s\n", err)
		log.flush()
	}
	return
}

var timeNow = time.Now

func (log *iLog) header() *buffer {
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "???"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	if line < 0 {
		line = 0
	}
	return log.formatHeader(file, line)
}

const digits = "0123456789"

func someDigits(fb *buffer, d int) {
	buf := fb.getBuf()
	pos := fb.getReadOffset()
	buf = buf[pos:]
	j := len(buf)
	for {
		j--
		buf[j] = digits[d%10]
		d /= 10
		if d == 0 {
			break
		}
	}
	ret := copy(buf, buf[j:])
	_ = fb.resize(ret+pos, 0)
	return
}

func str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func (log *iLog) formatHeader(file string, line int) *buffer {
	now := timeNow()
	if line < 0 {
		line = 0 // not a real line number, but acceptable to someDigits
	}
	fb := newBuffer()
	fb.Write(str2bytes(timeFormater(now)))
	fb.writeByte(space)
	fb.writeByte(leftBracket)
	fb.writeByte(severityChar[log.severity])
	fb.writeByte(space)
	fb.Write(str2bytes(file))
	fb.writeByte(dotDot)
	someDigits(fb, line)
	fb.writeByte(rightBracket)
	fb.writeByte(space)
	return fb
}

func (log *iLog) output(buf []byte) {
	log.Lock()
	if log.Writer == nil {
		log.create()
	}
	_, _ = log.write(buf)
	log.Unlock()
}

func (log *iLog) println(args ...interface{}) {
	fb := log.header()
	fmt.Fprintln(fb, args...)
	log.output(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) printf(format string, args ...interface{}) {
	fb := log.header()
	fmt.Fprintf(fb, format, args...)
	log.output(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) flush() {
	if log == nil {
		return
	}
	log.Lock()
	if log.Writer != nil {
		log.Writer.Flush()
	}
	log.Unlock()
}

func (log *iLog) close() {
	log.Lock()
	if log.Writer != nil {
		log.Writer.Close()
	}
	log.Unlock()
}

type decision bool

// stacks is a wrapper for runtime.Stack that attempts to recover the data for all goroutines.
func stacks(all bool) []byte {
	// We don't know how big the traces are, so grow a few times if they don't fit. Start large, though.
	n := 10000
	if all {
		n = 100000
	}
	var trace []byte
	for i := 0; i < 5; i++ {
		trace = make([]byte, n)
		nbytes := runtime.Stack(trace, all)
		if nbytes < len(trace) {
			return trace[:nbytes]
		}
		n *= 2
	}
	return trace
}

func checkDir() {
	_, err := os.Stat(logCfg.logDir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(logCfg.logDir, os.ModePerm)
			if err == nil {
				return
			}
		}
	}
	return
}

func flushThread() {
	if logCfg.flushInterval < 0 {
		logCfg.flushInterval = 30
	}
	duration := time.Duration(logCfg.flushInterval) * time.Second
	c := time.NewTicker(duration)
	defer c.Stop()
	for {
		c.Reset(duration)
		select {
		case <-c.C:
			Flush()
		}
	}
}
