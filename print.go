package jlog

import (
	"fmt"
	lumberjack "github.com/junjiefly/lumberjack"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	kind       logKind
	fileSuffix string
	sync.RWMutex
	Writer syncWriter
}

type fileWriter struct {
	logger   *lumberjack.Logger
	external []io.Writer
	ready    int32
}

func (w *fileWriter) Write(p []byte) (int, error) {
	n, err := w.logger.Write(p)
	if err == nil || n > 0 {
		atomic.StoreInt32(&w.ready, 1)
	}
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return n, err
	}
	for _, writer := range w.external {
		n, err = writer.Write(p)
		if err == nil && n != len(p) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return n, err
		}
	}
	return len(p), nil
}

type externalWriter struct {
	mu      sync.Mutex
	writers []io.Writer
}

func newExternalWriter(writers []io.Writer) *externalWriter {
	return &externalWriter{writers: writers}
}

func (w *externalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.writers) == 0 {
		return 0, nil
	}
	for _, writer := range w.writers {
		n, err := writer.Write(p)
		if err != nil {
			return n, err
		}
		if n != len(p) {
			return n, io.ErrShortWrite
		}
	}
	return len(p), nil
}

func (w *externalWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var firstErr error
	for _, writer := range w.writers {
		flusher, ok := writer.(interface{ Flush() error })
		if !ok {
			continue
		}
		if err := flusher.Flush(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (w *externalWriter) Close() error {
	return w.Flush()
}

func (w *externalWriter) SetOutput(localWrite bool, writers []io.Writer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writers = writers
}

func (w *fileWriter) Flush() error {
	var firstErr error
	if atomic.LoadInt32(&w.ready) == 0 {
		return firstErr
	}
	if err := w.logger.Flush(); err != nil {
		firstErr = err
	}
	for _, writer := range w.external {
		flusher, ok := writer.(interface{ Flush() error })
		if !ok {
			continue
		}
		if err := flusher.Flush(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Close flushes the lumberjack logger and releases its file handle.
func (w *fileWriter) Close() error {
	err := w.Flush()
	if closeErr := w.logger.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func (w *fileWriter) SetOutput(localWrite bool, writers []io.Writer) {
	w.logger.SetOutput(localWrite, writers)
}

func (log *iLog) create() {
	cfgMu.RLock()
	cfg := logCfg
	cfgMu.RUnlock()

	fileSuffix := log.fileSuffix
	if fileSuffix == "" {
		fileSuffix = logKindFileSuffix[log.kind]
	}

	if !cfg.LocalWrite {
		external := make([]io.Writer, 0, len(cfg.Writers)+1)
		external = append(external, cfg.Writers...)
		if cfg.Stdout {
			external = append(external, os.Stdout)
		}
		log.Writer = newExternalWriter(external)
		return
	}

	checkDir(cfg.LogDir)
	logger := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, cfg.FileName+fileSuffix),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}
	fw := &fileWriter{logger: logger}
	writers := make([]io.Writer, 0, len(cfg.Writers)+1)
	writers = append(writers, cfg.Writers...)
	if cfg.Stdout {
		writers = append(writers, os.Stdout)
	}
	logger.SetOutput(true, nil)
	fw.external = writers
	log.Writer = fw
}

func (log *iLog) write(p []byte) (n int, err error) {
	n, err = log.Writer.Write(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: log can't be done because of error: %s\n", err)
		// output() already holds log's lock; flush() would re-lock and deadlock.
		_ = log.Writer.Flush()
	}
	return
}

var timeNow = time.Now

func (log *iLog) header(l Level) *buffer {
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "???"
		line = 1
	} else {
		file = trimCallerPath(file)
	}
	if line < 0 {
		line = 0
	}
	return log.formatHeader(l, file, line)
}

func trimCallerPath(file string) string {
	if slash := strings.LastIndexAny(file, "/\\"); slash >= 0 {
		return file[slash+1:]
	}
	return file
}

func someDigits(fb *buffer, d int64) {
	var tmp [24]byte
	fb.Write(strconv.AppendInt(tmp[:0], d, 10))
}

func writeTextField(fb *buffer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '\n' && s[i] != '\r' {
			continue
		}
		if i > start {
			fb.Write(str2bytes(s[start:i]))
		}
		if s[i] == '\n' {
			fb.Write(str2bytes(`\n`))
		} else {
			fb.Write(str2bytes(`\r`))
		}
		start = i + 1
	}
	if start == 0 {
		fb.Write(str2bytes(s))
		return
	}
	if start < len(s) {
		fb.Write(str2bytes(s[start:]))
	}
}

func str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func (log *iLog) formatHeader(l Level, file string, line int) *buffer {
	now := timeNow()
	if line < 0 {
		line = 0 // not a real line number, but acceptable to someDigits
	}
	fb := newBuffer()
	fb.writeTime(now)
	fb.writeByte(space)

	fb.writeByte(leftBracket)
	fb.writeByte(levelChar[l])
	fb.writeByte(space)
	fb.Write(str2bytes(file))
	fb.writeByte(dotDot)
	someDigits(fb, int64(line))
	fb.writeByte(rightBracket)
	fb.writeByte(space)
	return fb
}

func (log *iLog) output(buf []byte) {
	log.Lock()
	if atomic.LoadInt32(&shuttingDown) != 0 {
		log.Unlock()
		return
	}
	log.outputLocked(buf)
	log.Unlock()
}

func (log *iLog) outputForced(buf []byte) {
	log.Lock()
	log.outputLocked(buf)
	log.Unlock()
}

func (log *iLog) outputLine(buf []byte) {
	fb := newBuffer()
	fb.Write(buf)
	if b := fb.bytes(); len(b) == 0 || b[len(b)-1] != '\n' {
		fb.writeByte('\n')
	}
	log.outputForced(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) outputLocked(buf []byte) {
	if log.Writer == nil {
		log.create()
	}
	_, _ = log.write(buf)
}

func (log *iLog) fatalPrintln(args ...interface{}) {
	fb := log.header(FatalLevel)
	fmt.Fprintln(fb, args...)
	if b := fb.bytes(); len(b) == 0 || b[len(b)-1] != '\n' {
		fb.writeByte('\n')
	}
	log.outputForced(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) fatalPrintf(format string, args ...interface{}) {
	fb := log.header(FatalLevel)
	fmt.Fprintf(fb, format, args...)
	if b := fb.bytes(); len(b) == 0 || b[len(b)-1] != '\n' {
		fb.writeByte('\n')
	}
	log.outputForced(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) println(l Level, args ...interface{}) {
	fb := log.header(l)
	fmt.Fprintln(fb, args...)
	if b := fb.bytes(); len(b) == 0 || b[len(b)-1] != '\n' {
		fb.writeByte('\n')
	}
	log.output(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) printf(l Level, format string, args ...interface{}) {
	fb := log.header(l)
	fmt.Fprintf(fb, format, args...)
	if b := fb.bytes(); len(b) == 0 || b[len(b)-1] != '\n' {
		fb.writeByte('\n')
	}
	log.output(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) flush() error {
	if log == nil {
		return nil
	}
	log.Lock()
	defer log.Unlock()
	if log.Writer == nil {
		return nil
	}
	return log.Writer.Flush()
}

func (log *iLog) close() error {
	log.Lock()
	defer log.Unlock()
	if log.Writer == nil {
		return nil
	}
	err := log.Writer.Close()
	log.Writer = nil
	return err
}

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

func checkDir(dir string) {
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.MkdirAll(dir, os.ModePerm)
		}
	}
}

var (
	flushStop     chan struct{}
	flushDone     chan struct{}
	flushInterval int
	flushMu       sync.Mutex
)

func stopFlushThread() {
	flushMu.Lock()
	if flushStop == nil {
		flushMu.Unlock()
		return
	}

	stop := flushStop
	done := flushDone
	close(stop)
	flushStop = nil
	flushDone = nil
	flushInterval = 0
	flushMu.Unlock()
	<-done
}

func restartFlushThread(interval int) {
	if interval <= 0 {
		interval = 30
	}
	flushMu.Lock()

	for {
		if flushStop == nil {
			break
		}
		if flushInterval == interval {
			flushMu.Unlock()
			return
		}
		stop := flushStop
		done := flushDone
		close(stop)
		flushStop = nil
		flushDone = nil
		flushInterval = 0
		flushMu.Unlock()
		<-done
		flushMu.Lock()
	}

	flushInterval = interval
	stop := make(chan struct{})
	done := make(chan struct{})
	flushStop = stop
	flushDone = done
	go func() {
		defer close(done)
		flushThread(stop, interval)
	}()
	flushMu.Unlock()
}

func flushThread(stop chan struct{}, interval int) {
	c := time.NewTicker(time.Duration(interval) * time.Second)
	defer c.Stop()
	for {
		select {
		case <-stop:
			return
		case <-c.C:
			Flush()
		}
	}
}
