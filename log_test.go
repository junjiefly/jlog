package jlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *memWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *memWriter) Flush() error                { return nil }
func (w *memWriter) Close() error                { return nil }
func (w *memWriter) SetOutput(bool, []io.Writer) {}

func (w *memWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func tempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "jlog")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error)   { return 0, errors.New("simulated write failure") }
func (errWriter) Flush() error                { return nil }
func (errWriter) Close() error                { return nil }
func (errWriter) SetOutput(bool, []io.Writer) {}

type errFlushWriter struct{}

func (errFlushWriter) Write(p []byte) (int, error) { return len(p), nil }
func (errFlushWriter) Flush() error                { return errors.New("simulated flush failure") }
func (errFlushWriter) Close() error                { return nil }
func (errFlushWriter) SetOutput(bool, []io.Writer) {}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return 1, nil
}

func (shortWriter) Flush() error                { return nil }
func (shortWriter) Close() error                { return nil }
func (shortWriter) SetOutput(bool, []io.Writer) {}

func TestLevelThreshold(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w
	SetLevel(WarnLevel)

	Debugf("d=%d", 1)
	Debugln("dl")
	Infof("i=%d", 1)
	Infoln("il")
	if w.String() != "" {
		t.Fatalf("below-threshold records leaked: %q", w.String())
	}

	Warnf("w=%d", 1)
	Warnln("wl")
	Errorf("e=%d", 1)
	Errorln("el")
	for _, want := range []string{"w=1", "wl", "e=1", "el"} {
		if !strings.Contains(w.String(), want) {
			t.Fatalf("missing %q in %q", want, w.String())
		}
	}
	SetLevel(InfoLevel)
}

func TestDefaultLevelAndStructured(t *testing.T) {
	if err := InitWithDefaultConfig(); err != nil {
		t.Fatal(err)
	}
	w := &memWriter{}
	loggers[appLog].Writer = w

	if Debug() != nil {
		t.Fatal("Debug() should be nil at InfoLevel threshold")
	}
	if Info() == nil {
		t.Fatal("Info() should be non-nil at InfoLevel threshold")
	}
	Info().Str("key", "value").Msg("structured log")

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", w.String(), err)
	}
	if m["level"] != "info" || m["key"] != "value" || m["msg"] != "structured log" {
		t.Fatalf("fields mismatch: %#v", m)
	}
}

func TestErrField(t *testing.T) {
	oldWriter := loggers[appLog].Writer
	defer func() { loggers[appLog].Writer = oldWriter }()

	w := &memWriter{}
	loggers[appLog].Writer = w

	Info().Err(errors.New("boom")).Msg("failed")
	Info().Err(nil).Msg("ok")

	if !strings.Contains(w.String(), `"error":"boom"`) {
		t.Fatalf("error field missing: %q", w.String())
	}
	if lines := strings.Split(strings.TrimSpace(w.String()), "\n"); len(lines) != 2 || strings.Contains(lines[1], `"error":`) {
		t.Fatalf("nil error should be omitted: %q", w.String())
	}
}

func TestFlushAndShutdownReturnErrors(t *testing.T) {
	oldWriter := loggers[appLog].Writer
	oldShuttingDown := atomic.LoadInt32(&shuttingDown)
	defer func() {
		loggers[appLog].Writer = oldWriter
		atomic.StoreInt32(&shuttingDown, oldShuttingDown)
	}()

	loggers[appLog].Writer = errFlushWriter{}
	if err := Flush(); err == nil {
		t.Fatal("Flush did not return writer error")
	}
	if err := Shutdown(); err == nil {
		t.Fatal("Shutdown did not return writer error")
	}
}

func TestConfigNormalizationDefaults(t *testing.T) {
	got := normalizeConfig(Config{Level: FatalLevel})
	if got.LogDir != "log" || got.FileName != program {
		t.Fatalf("unexpected path defaults: %#v", got)
	}
	if got.FlushInterval != 30 || got.MaxSize != 100 || got.MaxBackups != 0 || got.MaxAge != 0 {
		t.Fatalf("unexpected numeric defaults: %#v", got)
	}
	if got := normalizeConfig(Config{FlushInterval: int(maxInt)}); got.FlushInterval != int(maxInt/int64(time.Second)) {
		t.Fatalf("FlushInterval overflow guard failed: got=%d", got.FlushInterval)
	}
	if got.Level != ErrorLevel {
		t.Fatalf("level not clamped: %#v", got.Level)
	}
	if overflowed := normalizeConfig(Config{MaxSize: 1<<63 - 1}); overflowed.MaxSize != maxConfiguredSize {
		t.Fatalf("MaxSize overflow guard failed: got=%d want=%d", overflowed.MaxSize, maxConfiguredSize)
	}
	if got := normalizeConfig(Config{MaxAge: int(maxInt)}); got.MaxAge != int(maxInt/int64(24*time.Hour)) {
		t.Fatalf("MaxAge overflow guard failed: got=%d", got.MaxAge)
	}
}

func TestConfigWritersCopied(t *testing.T) {
	writer := &memWriter{}
	cfg := Config{Writers: []io.Writer{writer}}
	got := normalizeConfig(cfg)
	cfg.Writers[0] = nil
	if got.Writers[0] != io.Writer(writer) {
		t.Fatalf("writers slice was not copied: %#v", got.Writers)
	}
}

func restoreCustomLogs(t *testing.T, logs []*CustomLog, names, files map[string]struct{}) {
	t.Helper()
	customLogMu.Lock()
	customLogs = logs
	customLogNames = names
	customLogFiles = files
	customLogMu.Unlock()
}

func cloneCustomLogSet(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for key := range src {
		dst[key] = struct{}{}
	}
	return dst
}

func saveCustomLogs() ([]*CustomLog, map[string]struct{}, map[string]struct{}) {
	customLogMu.RLock()
	defer customLogMu.RUnlock()
	return customLogs,
		cloneCustomLogSet(customLogNames),
		cloneCustomLogSet(customLogFiles)
}

func TestCustomLogEvent(t *testing.T) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	defer restoreCustomLogs(t, oldLogs, oldNames, oldFiles)

	customLog, err := RegisterCustomLog(CustomLogConfig{
		Name:       "audit",
		FileSuffix: ".jsonl",
		Fields: map[string]interface{}{
			"service": "payments \"v2\"\nEU",
			"version": 2,
			"ok":      true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if customLog.fileSuffix != ".audit.jsonl" {
		t.Fatalf("fileSuffix=%q, want .audit.jsonl", customLog.fileSuffix)
	}

	w := &memWriter{}
	customLog.logger.Writer = w
	SetLevel(ErrorLevel)
	defer SetLevel(InfoLevel)

	customLog.Event().
		Str("action", "refund").
		Int64("amount", 1999).
		Msg("")

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", w.String(), err)
	}
	if m["service"] != "payments \"v2\"\nEU" || m["version"] != float64(2) || m["ok"] != true {
		t.Fatalf("fixed fields mismatch: %#v", m)
	}
	if m["action"] != "refund" || m["amount"] != float64(1999) {
		t.Fatalf("event fields mismatch: %#v", m)
	}
	if _, hasLevel := m["level"]; hasLevel {
		t.Fatalf("custom log should not contain level: %#v", m)
	}
}

func TestCustomLogLifecycle(t *testing.T) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	oldCfg := logCfg
	defer func() {
		restoreCustomLogs(t, oldLogs, oldNames, oldFiles)
		cfgMu.Lock()
		logCfg = oldCfg
		cfgMu.Unlock()
	}()

	customLog, err := RegisterCustomLog(CustomLogConfig{Name: "billing", FileSuffix: ".ndjson"})
	if err != nil {
		t.Fatal(err)
	}
	customLog.logger.Writer = &memWriter{}

	dir := tempDir(t)
	defer os.RemoveAll(dir)
	InitWithConfig(Config{LogDir: dir, FileName: "lifecycle", FlushInterval: 30, LocalWrite: true})
	if customLog.logger.Writer != nil {
		t.Fatal("re-initialization did not close custom log")
	}

	customLog.logger.create()
	writer, ok := customLog.logger.Writer.(*fileWriter)
	if !ok {
		t.Fatalf("writer type = %T, want *fileWriter", customLog.logger.Writer)
	}
	if want := filepath.Join(dir, "lifecycle.billing.ndjson"); writer.logger.Filename != want {
		t.Fatalf("custom log path=%q want=%q", writer.logger.Filename, want)
	}
	customLog.logger.close()

	w := &memWriter{}
	customLog.logger.Writer = w
	customLog.Event().Str("order", "o-1").Msg("")
	if !strings.Contains(w.String(), `"order":"o-1"`) {
		t.Fatalf("custom event missing after re-init: %q", w.String())
	}
}

func TestCustomLogValidation(t *testing.T) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	defer restoreCustomLogs(t, oldLogs, oldNames, oldFiles)

	cases := []CustomLogConfig{
		{Name: "", FileSuffix: ".audit"},
		{Name: "bad/name", FileSuffix: ".audit"},
		{Name: "audit", FileSuffix: ""},
		{Name: "audit", FileSuffix: "audit"},
		{Name: "audit", FileSuffix: ".bad/name"},
		{Name: "audit", FileSuffix: ".jsonl."},
		{Name: "audit", FileSuffix: ".audit", Fields: map[string]interface{}{"bad": make(chan int)}},
		{Name: "audit", FileSuffix: ".audit", Fields: map[string]interface{}{"time": "fixed"}},
		{Name: "audit", FileSuffix: ".audit", Fields: map[string]interface{}{"level": "fixed"}},
	}
	for _, cfg := range cases {
		if _, err := RegisterCustomLog(cfg); err == nil {
			t.Fatalf("expected error for %#v", cfg)
		}
	}
}

func TestCustomLogFieldsCannotBeOverridden(t *testing.T) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	defer restoreCustomLogs(t, oldLogs, oldNames, oldFiles)

	customLog, err := RegisterCustomLog(CustomLogConfig{
		Name:       "protected",
		FileSuffix: ".jsonl",
		Fields: map[string]interface{}{
			"service": "fixed",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := &memWriter{}
	customLog.logger.Writer = w

	customLog.Event().
		Str("service", "event").
		Str("action", "refund").
		Msg("")

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", w.String(), err)
	}
	if m["service"] != "fixed" || m["action"] != "refund" {
		t.Fatalf("field precedence mismatch: %#v", m)
	}
	if got := strings.Count(w.String(), `"service":`); got != 1 {
		t.Fatalf("service field count=%d, output=%q", got, w.String())
	}
}

func TestCustomLongTimeFormatRemainsValidJSON(t *testing.T) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	defer restoreCustomLogs(t, oldLogs, oldNames, oldFiles)
	defer TimeFormat("2006-01-02T15:04:05.000")
	// A format this long still overflows the fixed-field fragment, but leaves
	// room for event fields instead of producing a multi-megabyte dump.
	TimeFormat(strings.Repeat("x", maxBufSize-128))

	customLog, err := RegisterCustomLog(CustomLogConfig{
		Name:       "long-time",
		FileSuffix: ".jsonl",
		Fields: map[string]interface{}{
			"service": strings.Repeat("y", 128),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := &memWriter{}
	customLog.logger.Writer = w
	customLog.Event().Str("action", "refund").Msg("done")

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("long time format produced invalid JSON: %v", err)
	}
	if _, ok := m["service"]; ok {
		t.Fatal("unsplittable fixed fields should be dropped atomically")
	}
	if m["action"] != "refund" || m["msg"] != "done" {
		t.Fatalf("event fields mismatch: %#v", m)
	}
}

func TestNormalizeConfigFiltersInvalidWriters(t *testing.T) {
	var typedNil *memWriter
	got := normalizeConfig(Config{
		Writers: []io.Writer{nil, typedNil, &memWriter{}},
	})
	if len(got.Writers) != 1 || got.Writers[0] == nil {
		t.Fatalf("invalid writers were not filtered: %#v", got.Writers)
	}
}

func TestExternalOnlyWriterDoesNotTouchDisk(t *testing.T) {
	oldCfg := logCfg
	defer func() {
		cfgMu.Lock()
		logCfg = oldCfg
		cfgMu.Unlock()
	}()

	root := tempDir(t)
	defer os.RemoveAll(root)
	dir := filepath.Join(root, "external-only-dir")
	w := &memWriter{}
	cfgMu.Lock()
	logCfg = normalizeConfig(Config{
		LogDir:     dir,
		FileName:   "external-only",
		LocalWrite: false,
		Writers:    []io.Writer{w},
	})
	cfgMu.Unlock()

	l := &iLog{kind: appLog}
	l.create()
	if _, err := l.write([]byte("external-data\n")); err != nil {
		t.Fatal(err)
	}
	l.flush()
	l.close()

	if _, err := os.Stat(filepath.Join(dir, "external-only.log")); !os.IsNotExist(err) {
		t.Fatalf("external-only config created local file: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("external-only config created log directory: %v", err)
	}
	if !strings.Contains(w.String(), "external-data") {
		t.Fatalf("external record missing: %q", w.String())
	}
	if strings.Contains(w.String(), "Log file opened at:") {
		t.Fatalf("external writer received lumberjack metadata: %q", w.String())
	}
}

func TestSetLevelAndShutdownBeforeInitHaveNoSideEffects(t *testing.T) {
	oldLevel := currentLevel
	oldCfg := logCfg
	oldShuttingDown := atomic.LoadInt32(&shuttingDown)
	oldAppWriter := loggers[appLog].Writer
	oldHTTPWriter := httpLog.logger.Writer
	defer func() {
		currentLevel = oldLevel
		logCfg = oldCfg
		atomic.StoreInt32(&shuttingDown, oldShuttingDown)
		loggers[appLog].Writer = oldAppWriter
		httpLog.logger.Writer = oldHTTPWriter
	}()

	currentLevel = int32(InfoLevel)
	logCfg = Config{}
	loggers[appLog].Writer = nil
	httpLog.logger.Writer = nil

	SetLevel(WarnLevel)
	if got := Level(currentLevel); got != WarnLevel {
		t.Fatalf("SetLevel did not update threshold: got=%v", got)
	}
	Shutdown()
	if loggers[appLog].Writer != nil || httpLog.logger.Writer != nil {
		t.Fatal("pre-init lifecycle calls created a logger")
	}
	Infof("after shutdown")
	if loggers[appLog].Writer != nil || httpLog.logger.Writer != nil {
		t.Fatal("logging after Shutdown lazily recreated a log")
	}
}

func TestCloseResetsWriter(t *testing.T) {
	l := &iLog{kind: appLog}
	l.Writer = &memWriter{}
	l.close()
	if l.Writer != nil {
		t.Fatal("closed logger is still reusable without re-initialization")
	}
}

func TestFlushBeforeFirstWriteIsSafe(t *testing.T) {
	old := logCfg
	dir := tempDir(t)
	defer os.RemoveAll(dir)
	cfgMu.Lock()
	logCfg = normalizeConfig(Config{
		LogDir:     dir,
		FileName:   "flush-before-write",
		LocalWrite: true,
	})
	cfgMu.Unlock()
	defer func() {
		cfgMu.Lock()
		logCfg = old
		cfgMu.Unlock()
	}()

	l := &iLog{kind: appLog}
	l.create()
	l.flush()
	if _, err := l.write([]byte("hello\n")); err != nil {
		t.Fatalf("write after safe flush failed: %v", err)
	}
	l.close()
	if err := os.Remove(filepath.Join(dir, "flush-before-write.log")); err != nil {
		t.Fatalf("file handle was not released on close: %v", err)
	}
}

func TestDisabledStructuredSafe(t *testing.T) {
	SetLevel(ErrorLevel)
	w := &memWriter{}
	loggers[appLog].Writer = w

	Debug().Str("k", "v").Msg("no panic")
	if w.String() != "" {
		t.Fatalf("disabled chain logged: %q", w.String())
	}
	SetLevel(InfoLevel)
}

func TestStructuredBaseKeysAreReserved(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w

	Info().
		Str("time", "bad").
		Str("level", "bad").
		Str("msg", "bad").
		Msg("good")

	out := w.String()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	for _, key := range []string{"time", "level", "msg"} {
		if got := strings.Count(out, `"`+key+`":`); got != 1 {
			t.Fatalf("base key %q appeared %d times: %q", key, got, out)
		}
	}
	if m["msg"] != "good" {
		t.Fatalf("msg mismatch: %#v", m["msg"])
	}
}

func TestEntryMethodsAfterMsgAreNoOps(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w

	e := Info()
	if e == nil {
		t.Fatal("Info() returned nil at InfoLevel")
	}
	e.Msg("first")
	e.Str("late", "value").Bytes("lateBytes", []byte("value")).Msg("late")

	var records []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(w.String()), "\n") {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON %q: %v", line, err)
		}
		records = append(records, m)
	}
	if len(records) != 1 || records[0]["msg"] != "first" {
		t.Fatalf("unexpected records after completed entry: %#v", records)
	}
}

func TestMsgOnExhaustedBufferRemainsValid(t *testing.T) {
	for _, remaining := range []int{0, 1} {
		w := &memWriter{}
		loggers[appLog].Writer = w

		e := Info()
		if e == nil {
			t.Fatal("Info() returned nil at InfoLevel")
		}
		e.buf.roff = maxBufSize - remaining
		e.Msg("dropped")

		var m map[string]interface{}
		if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
			t.Fatalf("remaining=%d produced invalid JSON %q: %v", remaining, w.String(), err)
		}
		if len(m) != 0 {
			t.Fatalf("remaining=%d expected empty object: %#v", remaining, m)
		}
	}
}

func TestLocalWriteFlushesWhenExternalWriterFails(t *testing.T) {
	oldCfg := logCfg
	defer func() {
		cfgMu.Lock()
		logCfg = oldCfg
		cfgMu.Unlock()
	}()

	dir := filepath.Join(os.TempDir(), fmt.Sprintf("jlog-external-error-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	cfgMu.Lock()
	logCfg = normalizeConfig(Config{
		LogDir:     dir,
		FileName:   "external-error",
		LocalWrite: true,
		Writers:    []io.Writer{errWriter{}},
	})
	cfgMu.Unlock()

	l := &iLog{kind: appLog}
	l.create()
	if _, err := l.write([]byte("local-data\n")); err == nil {
		t.Fatal("expected external writer error")
	}
	l.close()

	data, err := ioutil.ReadFile(filepath.Join(dir, "external-error.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "local-data") {
		t.Fatalf("local data was not flushed after external writer failure: %q", string(data))
	}
}

func TestLocalWriteDetectsShortExternalWrite(t *testing.T) {
	oldCfg := logCfg
	defer func() {
		cfgMu.Lock()
		logCfg = oldCfg
		cfgMu.Unlock()
	}()

	dir := tempDir(t)
	defer os.RemoveAll(dir)
	cfgMu.Lock()
	logCfg = normalizeConfig(Config{
		LogDir:     dir,
		FileName:   "external-short",
		LocalWrite: true,
		Writers:    []io.Writer{shortWriter{}},
	})
	cfgMu.Unlock()

	l := &iLog{kind: appLog}
	l.create()
	if _, err := l.write([]byte("short-external-data\n")); err == nil {
		t.Fatal("expected short external write to fail")
	}
	l.close()
}

func TestLocalWriteExternalWriterGetsRecordsOnly(t *testing.T) {
	oldCfg := logCfg
	defer func() {
		cfgMu.Lock()
		logCfg = oldCfg
		cfgMu.Unlock()
	}()

	dir := tempDir(t)
	defer os.RemoveAll(dir)
	w := &memWriter{}
	cfgMu.Lock()
	logCfg = normalizeConfig(Config{
		LogDir:     dir,
		FileName:   "local-external",
		LocalWrite: true,
		Writers:    []io.Writer{w},
	})
	cfgMu.Unlock()

	l := &iLog{kind: appLog}
	l.create()
	if _, err := l.write([]byte("local-external-data\n")); err != nil {
		t.Fatal(err)
	}
	l.flush()
	l.close()

	out := w.String()
	if !strings.Contains(out, "local-external-data") {
		t.Fatalf("external record missing: %q", out)
	}
	if strings.Contains(out, "Log file opened at:") {
		t.Fatalf("local writer metadata leaked to external writer: %q", out)
	}
}

func TestTextHeaderLevels(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w

	Infof("info-line")
	Errorf("error-line")
	loggers[appLog].printf(FatalLevel, "fatal-line")
	for _, want := range []string{"[I ", "[E ", "[F "} {
		if !strings.Contains(w.String(), want) {
			t.Fatalf("level char %q missing in %q", want, w.String())
		}
	}
}

func TestFatalBypassesShutdownGate(t *testing.T) {
	oldShuttingDown := atomic.LoadInt32(&shuttingDown)
	oldWriter := loggers[appLog].Writer
	defer func() {
		atomic.StoreInt32(&shuttingDown, oldShuttingDown)
		loggers[appLog].Writer = oldWriter
	}()

	atomic.StoreInt32(&shuttingDown, 1)
	w := &memWriter{}
	loggers[appLog].Writer = w

	loggers[appLog].fatalPrintln("fatal-after-shutdown")
	loggers[appLog].fatalPrintf("fatal-format=%d", 1)

	if !strings.Contains(w.String(), "fatal-after-shutdown") || !strings.Contains(w.String(), "fatal-format=1") {
		t.Fatalf("fatal records were dropped during shutdown: %q", w.String())
	}
}

func TestHttpsNoLevelField(t *testing.T) {
	w := &memWriter{}
	httpLog.logger.Writer = w
	r := httptest.NewRequest("GET", "http://example.com/path", nil)
	r.RemoteAddr = "1.2.3.4:5678"

	Https(r, "req-1", "example.com", timeNow().UnixNano(), 200, "", 128, nil)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", w.String(), err)
	}
	if _, ok := m["level"]; ok {
		t.Fatalf("http record should not have level field: %#v", m)
	}
	if m["reqId"] != "req-1" {
		t.Fatalf("reqId mismatch: %#v", m)
	}
}

func TestHTTPUsesBuiltInCustomLog(t *testing.T) {
	customLogMu.RLock()
	found := false
	for _, log := range customLogs {
		if log == httpLog {
			found = true
			break
		}
	}
	customLogMu.RUnlock()
	if !found {
		t.Fatal("built-in HTTP log is not managed as a custom log")
	}

	oldCfg := logCfg
	dir := tempDir(t)
	defer os.RemoveAll(dir)
	cfgMu.Lock()
	logCfg = normalizeConfig(Config{LogDir: dir, FileName: "built-in", LocalWrite: true})
	cfgMu.Unlock()
	defer func() {
		cfgMu.Lock()
		logCfg = oldCfg
		cfgMu.Unlock()
	}()

	httpLog.logger.create()
	writer, ok := httpLog.logger.Writer.(*fileWriter)
	if !ok {
		t.Fatalf("writer type = %T, want *fileWriter", httpLog.logger.Writer)
	}
	if want := filepath.Join(dir, "built-in.http"); writer.logger.Filename != want {
		t.Fatalf("HTTP log path=%q want=%q", writer.logger.Filename, want)
	}
	httpLog.logger.close()
}

func TestHTTPNilRequestDoesNotPanic(t *testing.T) {
	w := &memWriter{}
	httpLog.logger.Writer = w

	Http(nil, "req", "host", timeNow().UnixNano(), 200, "", 1, nil)
	Https(nil, "req", "host", timeNow().UnixNano(), 200, "", 1, nil)

	if w.String() != "" {
		t.Fatalf("nil request logged unexpectedly: %q", w.String())
	}
}

func TestHTTPRemoteHostIPv6(t *testing.T) {
	w := &memWriter{}
	httpLog.logger.Writer = w
	r := httptest.NewRequest("GET", "http://example.com/path", nil)
	r.RemoteAddr = "[2001:db8::1]:443"

	Https(r, "req-ipv6", "example.com", timeNow().UnixNano(), 200, "", 1, nil)

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", w.String(), err)
	}
	if m["remote"] != "2001:db8::1" {
		t.Fatalf("remote=%v, want 2001:db8::1", m["remote"])
	}
}

func TestInitWithConfigReplacesActiveWriter(t *testing.T) {
	oldLevel := currentLevel
	oldCfg := logCfg
	oldShuttingDown := atomic.LoadInt32(&shuttingDown)
	oldAppWriter := loggers[appLog].Writer
	oldHTTPWriter := httpLog.logger.Writer
	defer func() {
		currentLevel = oldLevel
		logCfg = oldCfg
		atomic.StoreInt32(&shuttingDown, oldShuttingDown)
		loggers[appLog].Writer = oldAppWriter
		httpLog.logger.Writer = oldHTTPWriter
	}()

	first := &memWriter{}
	second := &memWriter{}
	loggers[appLog].Writer = first
	httpLog.logger.Writer = &memWriter{}
	dir := tempDir(t)
	defer os.RemoveAll(dir)

	InitWithConfig(Config{
		LogDir:        dir,
		FileName:      "replace-test",
		FlushInterval: 30,
		LocalWrite:    true,
	})
	if loggers[appLog].Writer != nil || httpLog.logger.Writer != nil {
		t.Fatal("re-initialization did not reset old logs")
	}

	loggers[appLog].Writer = second
	Infof("after-reinit")
	if !strings.Contains(second.String(), "after-reinit") {
		t.Fatalf("record missing after re-init: %q", second.String())
	}
	loggers[appLog].close()
	loggers[appLog].create()
	writer, ok := loggers[appLog].Writer.(*fileWriter)
	if !ok {
		t.Fatalf("writer type = %T, want *fileWriter", loggers[appLog].Writer)
	}
	if want := filepath.Join(dir, "replace-test.log"); writer.logger.Filename != want {
		t.Fatalf("new log path=%q want=%q", writer.logger.Filename, want)
	}

	Shutdown()
}

func TestJSONEscapingValid(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w
	Info().Str("ua", `Mozilla/5.0 "v" \ path`).Msg("line1\nline2 \"q\" \\done")

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("invalid JSON %q: %v", w.String(), err)
	}
	if m["ua"] != `Mozilla/5.0 "v" \ path` || m["msg"] != "line1\nline2 \"q\" \\done" {
		t.Fatalf("roundtrip mismatch: %#v", m)
	}
}

func TestInvalidUTF8JSON(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w
	Info().Str("data", "\xff\xfe").Msg("invalid utf8")

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("invalid UTF-8 produced invalid JSON %q: %v", w.String(), err)
	}
	if m["data"] != "\uFFFD\uFFFD" {
		t.Fatalf("invalid UTF-8 roundtrip mismatch: %#v", m["data"])
	}
}

func TestBufferTruncationSafe(t *testing.T) {
	fb := newBuffer()
	defer freeBuffer(fb)
	fb.roff = maxBufSize - 10
	caller := []byte("0123456789ABCDEFGHIJ")

	n, err := fb.Write(caller)
	if string(caller) != "0123456789ABCDEFGHIJ" {
		t.Fatalf("caller buffer mutated: %q", caller)
	}
	if n != 9 || err != nil {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if got := string(fb.bytes()); !strings.HasSuffix(got, "012345678") {
		t.Fatalf("truncated write changed input bytes: %q", got)
	}
	if fb.remaining() != 1 {
		t.Fatalf("truncated write did not reserve newline slot: remaining=%d", fb.remaining())
	}
}

func TestWriteErrorDoesNotDeadlock(t *testing.T) {
	l := &iLog{kind: appLog}
	l.Writer = errWriter{}
	done := make(chan struct{})

	go func() {
		l.output([]byte("hello"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: iLog.output did not return after a write error")
	}
}

func TestFatalStackTruncationRemainsOneLine(t *testing.T) {
	l := &iLog{kind: appLog}
	l.Writer = &memWriter{}

	l.outputLine([]byte(strings.Repeat("a", maxBufSize+16)))
	l.outputLine([]byte("after-truncation\n"))

	out := l.Writer.(*memWriter).String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("truncated stack changed line count: %d", len(lines))
	}
	if !strings.HasSuffix(lines[0], strings.Repeat("a", 16)) || lines[1] != "after-truncation" {
		t.Fatalf("unexpected truncated stack output: %q", out)
	}
}

func TestTimeFormatAllCases(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	cases := []struct {
		tm   time.Time
		want string
	}{
		{time.Date(2024, 2, 14, 16, 39, 33, 261456789, loc), "2024-02-14T16:39:33.261"},
		{time.Date(2024, 2, 14, 16, 39, 33, 261000000, loc), "2024-02-14T16:39:33.261"},
		{time.Date(2024, 2, 14, 16, 39, 33, 0, loc), "2024-02-14T16:39:33.000"},
		{time.Date(2024, 2, 14, 16, 39, 33, 261456789, loc).UTC(), "2024-02-14T08:39:33.261"},
	}
	for i, c := range cases {
		if got := c.tm.Format(defaultTimeFormat); got != c.want {
			t.Fatalf("case %d: got %q want %q", i, got, c.want)
		}
	}
}

func TestSomeDigitsNegative(t *testing.T) {
	fb := newBuffer()
	defer freeBuffer(fb)

	someDigits(fb, -5)
	if got := string(fb.bytes()); !strings.HasSuffix(got, "-5") {
		t.Fatalf("got %q", got)
	}
}

func TestBufferBoundaryHelpersAreSafe(t *testing.T) {
	fb := newBuffer()
	defer freeBuffer(fb)
	fb.roff = maxBufSize - 2
	fb.writeRaw([]byte("abcdef"))
	if fb.remaining() != 2 {
		t.Fatalf("writeRaw changed remaining space: %d", fb.remaining())
	}
	fb.roff = maxBufSize
	fb.woff = maxBufSize
	someDigits(fb, 12345)
}

func TestBufferAlwaysHasRoomForNewline(t *testing.T) {
	fb := newBuffer()
	defer freeBuffer(fb)
	payload := strings.Repeat("a", maxBufSize-1)
	fb.roff = 0
	n, err := fb.Write([]byte(payload))
	if err != nil || n != len(payload) {
		t.Fatalf("exact capacity write failed: n=%d err=%v", n, err)
	}
	if n, _ := fb.writeByte(lineBreak); n != 1 {
		t.Fatal("newline could not use reserved final slot")
	}
	if got := fb.bytes(); len(got) != maxBufSize || got[len(got)-1] != lineBreak {
		t.Fatalf("full record is not newline-terminated: len=%d", len(got))
	}
}

func TestSomeDigitsSmallRemainingNoPanic(t *testing.T) {
	fb := newBuffer()
	defer freeBuffer(fb)
	fb.roff = maxBufSize - 1
	fb.woff = maxBufSize - 1
	before := string(fb.bytes())

	someDigits(fb, minInt64)
	if got := string(fb.bytes()); got != before {
		t.Fatalf("number write changed exhausted buffer: before=%q after=%q", before, got)
	}
}

func TestSomeDigitsMinInt(t *testing.T) {
	fb := newBuffer()
	defer freeBuffer(fb)

	someDigits(fb, minInt64)
	if got := string(fb.bytes()); got != "-9223372036854775808" {
		t.Fatalf("MinInt64 was not formatted correctly: %q", got)
	}
}

func TestTrimCallerPath(t *testing.T) {
	cases := map[string]string{
		"main.go":                  "main.go",
		`C:\src\app\main.go`:       "main.go",
		"/home/user/app/main.go":   "main.go",
		`C:\src\app\weird/name.go`: "name.go",
	}
	for input, want := range cases {
		if got := trimCallerPath(input); got != want {
			t.Fatalf("trimCallerPath(%q)=%q want %q", input, got, want)
		}
	}
}

func TestTimeFormatChangeAndJSON(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w
	TimeFormat(time.RFC3339Nano)
	defer TimeFormat("2006-01-02T15:04:05.000")

	Info().Msg("custom time")
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("custom time produced invalid JSON %q: %v", w.String(), err)
	}
	if _, ok := m["time"]; !ok {
		t.Fatalf("time field missing: %#v", m)
	}
}

func TestTimeFormatJSONEscaping(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w
	TimeFormat("\"\n\\")
	defer TimeFormat("2006-01-02T15:04:05.000")

	Info().Msg("custom time")
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("custom time produced invalid JSON %q: %v", w.String(), err)
	}
	if m["time"] != "\"\n\\" {
		t.Fatalf("time=%#v, want %#v", m["time"], "\"\n\\")
	}
}

func TestTimeFormatPlainTextEscaping(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w
	TimeFormat("x\ny")
	defer TimeFormat("2006-01-02T15:04:05.000")

	Infof("record")
	out := w.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("custom time broke one-record-per-line: %q", out)
	}
	if !strings.Contains(out, `x\ny`) {
		t.Fatalf("custom time line break was not escaped: %q", out)
	}
}

func TestHttpNegativeCostNoPanic(t *testing.T) {
	httpLog.logger.Writer = &memWriter{}
	r := httptest.NewRequest("GET", "http://example.com/path", nil)
	r.RemoteAddr = "1.2.3.4:5678"

	start := timeNow().UnixNano() + int64(time.Hour)
	Http(r, "req-1", "example.com", start, 200, "", 128, nil)
}

func TestHttpPlainTextEscapesLineBreaks(t *testing.T) {
	w := &memWriter{}
	httpLog.logger.Writer = w
	r := httptest.NewRequest("POST", "http://example.com/path?q=1", nil)
	r.RequestURI = "/path?q=1\nfake-entry"
	r.Header.Set("User-Agent", "evil\r\nfake-entry")

	Http(r, "req-1", "example.com", timeNow().UnixNano(), 200, "", 128,
		errors.New("disk error\nfake-entry"))

	out := w.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("HTTP record was not one newline-terminated line: %q", out)
	}
	for _, want := range []string{`\nfake-entry`, `\r\nfake-entry`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing escaped sequence %q in %q", want, out)
		}
	}
}

func TestInfofNewline(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w

	Infof("first=%d", 1)
	Infof("second=%d", 2)
	lines := strings.Split(strings.TrimSuffix(w.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "first=1") || !strings.HasSuffix(lines[1], "second=2") {
		t.Fatalf("records not one per line: %q", w.String())
	}
}

func TestLongPrintlnRemainsOneLine(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w
	Infoln(strings.Repeat("a", maxBufSize+16))

	out := w.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("overlong record was split into %d lines", len(lines))
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("overlong record is not newline-terminated: %q", out[len(out)-8:])
	}
}

func TestEntryTruncationStillValidJSON(t *testing.T) {
	w := &memWriter{}
	loggers[appLog].Writer = w

	e := Info()
	e.Str("big", strings.Repeat("a", 2*1024*1024))
	e.Str("tail", "should be dropped")
	e.Msg("end")

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(w.String()), &m); err != nil {
		t.Fatalf("truncated entry produced invalid JSON (len=%d): %v", len(w.String()), err)
	}
	if _, ok := m["tail"]; ok {
		t.Fatal("field past capacity should be dropped")
	}
}

func TestInitNoGoroutineLeak(t *testing.T) {
	oldCfg := logCfg
	oldShuttingDown := atomic.LoadInt32(&shuttingDown)
	defer func() {
		cfgMu.Lock()
		logCfg = oldCfg
		cfgMu.Unlock()
		atomic.StoreInt32(&shuttingDown, oldShuttingDown)
	}()

	time.Sleep(300 * time.Millisecond)
	base := runtime.NumGoroutine()
	dir := tempDir(t)
	defer os.RemoveAll(dir)

	for i := 0; i < 3; i++ {
		InitWithConfig(Config{
			LogDir:        dir,
			FlushInterval: 30,
			FileName:      "leaktest",
			LocalWrite:    true,
		})
	}
	time.Sleep(400 * time.Millisecond)
	if delta := runtime.NumGoroutine() - base; delta > 1 {
		t.Fatalf("expected at most 1 flush goroutine after 3 InitWithConfig, delta=%d", delta)
	}

	Shutdown()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= base {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("flush goroutine did not stop after Shutdown: base=%d now=%d", base, runtime.NumGoroutine())
}

func TestFlushThreadRestartWaitsForOldThread(t *testing.T) {
	oldStop := flushStop
	oldDone := flushDone
	oldInterval := flushInterval
	defer func() {
		if flushStop != nil {
			close(flushStop)
			if flushDone != nil {
				<-flushDone
			}
		}
		flushStop = oldStop
		flushDone = oldDone
		flushInterval = oldInterval
	}()

	restartFlushThread(5)
	restartFlushThread(30)
	if flushStop == nil || flushDone == nil {
		t.Fatal("restarted flush thread is missing")
	}

	stopFlushThread()
	if flushStop != nil || flushDone != nil || flushInterval != 0 {
		t.Fatalf("flush thread state was not reset: stop=%v done=%v interval=%d", flushStop, flushDone, flushInterval)
	}
}
