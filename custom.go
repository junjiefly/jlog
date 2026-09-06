package jlog

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"sync"
)

// CustomLogConfig describes a dedicated custom structured log.
type CustomLogConfig struct {
	Name string

	// FileSuffix must start with a dot, for example ".audit" or ".jsonl".
	FileSuffix string

	// Fields are written to every record. Keys are sorted once at
	// registration. Supported values are nil, string, []byte, bool, signed
	// and unsigned integers, and float32/float64.
	Fields map[string]interface{}
}

// CustomLog is a separate rotating destination for structured records. Create
// it once during process startup; CustomLog is not safe to copy.
type CustomLog struct {
	name       string
	fileSuffix string
	fields     []byte
	fixedKeys  map[string]struct{}
	logger     iLog
}

var (
	customLogNames = make(map[string]struct{})
	customLogFiles = make(map[string]struct{})
	customLogs     []*CustomLog
	customLogMu    sync.RWMutex
)

var httpLog = newCustomLog("http", ".http", nil)

func init() {
	customLogs = append(customLogs, httpLog)
}

func newCustomLog(name, fileSuffix string, fields []byte) *CustomLog {
	return &CustomLog{
		name:       name,
		fileSuffix: fileSuffix,
		fields:     fields,
		logger:     iLog{fileSuffix: fileSuffix},
	}
}

// RegisterCustomLog adds a persistent custom structured log. Records use the same directory,
// rotation, flush, external writer, and stdout settings as the application
// log. The output file is <LogDir>/<FileName>.<Name><FileSuffix>.
func RegisterCustomLog(cfg CustomLogConfig) (*CustomLog, error) {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()

	if !validCustomLogName(cfg.Name) {
		return nil, errors.New("jlog: custom log name must contain only letters, digits, '.', '_' or '-'")
	}
	if !validCustomLogSuffix(cfg.FileSuffix) {
		return nil, errors.New("jlog: custom log file suffix must start with '.' and contain only letters, digits, '.', '_' or '-'")
	}
	if _, exists := cfg.Fields["level"]; exists {
		return nil, errors.New("jlog: fixed field key cannot replace level")
	}

	fields, err := encodeFixedFields(cfg.Fields)
	if err != nil {
		return nil, err
	}
	// Base keys are handled by entry.baseReserved; this map holds only fixed
	// caller keys.
	fixedKeys := make(map[string]struct{}, len(cfg.Fields))
	for key := range cfg.Fields {
		fixedKeys[key] = struct{}{}
	}

	fileSuffix := "." + cfg.Name + cfg.FileSuffix
	s := newCustomLog(cfg.Name, fileSuffix, fields)
	s.fixedKeys = fixedKeys

	customLogMu.Lock()
	defer customLogMu.Unlock()
	if _, exists := customLogNames[cfg.Name]; exists {
		return nil, errors.New("jlog: custom log name is already registered")
	}
	if _, exists := customLogFiles[fileSuffix]; exists {
		return nil, errors.New("jlog: custom log file suffix is already registered")
	}
	customLogNames[cfg.Name] = struct{}{}
	customLogFiles[fileSuffix] = struct{}{}
	customLogs = append(customLogs, s)
	return s, nil
}

// Event returns a JSON entry for this custom log. Custom-log records are not gated
// by SetLevel and do not add a level field.
func (s *CustomLog) Event() *entry {
	if s == nil {
		return nil
	}
	e := newEntry()
	e.logger = &s.logger
	e.buf = newBuffer()
	e.reserved = s.fixedKeys
	e.baseReserved = true
	e.buf.writeByte('{')
	e = e.time(timeNow())
	if len(s.fields) > 0 && e.buf.remaining() >= len(s.fields)+jsonStrHeadroom {
		e.buf.Write(s.fields)
	}
	return e
}

func validCustomLogName(name string) bool {
	return validCustomLogToken(name, false)
}

func validCustomLogSuffix(suffix string) bool {
	return len(suffix) > 1 && suffix[0] == '.' &&
		suffix[len(suffix)-1] != '.' && validCustomLogToken(suffix[1:], false)
}

func validCustomLogToken(s string, allowLeadingDot bool) bool {
	if s == "" {
		return false
	}
	if allowLeadingDot && s[0] == '.' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-' || c == '.':
		default:
			return false
		}
	}
	return true
}

func encodeFixedFields(fields map[string]interface{}) ([]byte, error) {
	if len(fields) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		if key == "" {
			return nil, errors.New("jlog: fixed field key cannot be empty")
		}
		if key == "time" || key == "msg" || key == "level" {
			return nil, errors.New("jlog: fixed field key cannot replace time, msg or level")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fb := newBuffer()
	defer freeBuffer(fb)
	for _, key := range keys {
		if fb.remaining() < minFieldHeadroom {
			return nil, errors.New("jlog: fixed fields are too large")
		}
		fb.writeJSONKey(key)
		if err := writeFixedFieldValue(fb, fields[key]); err != nil {
			return nil, err
		}
		if fb.remaining() < minFieldHeadroom {
			return nil, errors.New("jlog: fixed fields are too large")
		}
	}

	encoded := make([]byte, len(fb.bytes()))
	copy(encoded, fb.bytes())
	return encoded, nil
}

func writeFixedFieldValue(fb *buffer, val interface{}) error {
	switch v := val.(type) {
	case nil:
		fb.writeRaw(str2bytes("null"))
	case string:
		fb.writeJSONString(v)
	case []byte:
		fb.writeJSONString(bytes2str(v))
	case bool:
		if v {
			fb.writeRaw(str2bytes("true"))
		} else {
			fb.writeRaw(str2bytes("false"))
		}
	case int:
		writeFixedInt(fb, int64(v))
	case int8:
		writeFixedInt(fb, int64(v))
	case int16:
		writeFixedInt(fb, int64(v))
	case int32:
		writeFixedInt(fb, int64(v))
	case int64:
		writeFixedInt(fb, v)
	case uint:
		writeFixedUint(fb, uint64(v))
	case uint8:
		writeFixedUint(fb, uint64(v))
	case uint16:
		writeFixedUint(fb, uint64(v))
	case uint32:
		writeFixedUint(fb, uint64(v))
	case uint64:
		writeFixedUint(fb, v)
	case float32:
		writeFixedFloat(fb, float64(v), 32)
	case float64:
		writeFixedFloat(fb, v, 64)
	default:
		return errors.New("jlog: unsupported fixed field value type")
	}
	return nil
}

func writeFixedInt(fb *buffer, val int64) {
	var tmp [24]byte
	fb.writeRaw(strconv.AppendInt(tmp[:0], val, 10))
}

func writeFixedUint(fb *buffer, val uint64) {
	var tmp [24]byte
	fb.writeRaw(strconv.AppendUint(tmp[:0], val, 10))
}

func writeFixedFloat(fb *buffer, val float64, bits int) {
	switch {
	case math.IsNaN(val):
		fb.writeJSONString("NaN")
	case math.IsInf(val, 1):
		fb.writeJSONString("+Inf")
	case math.IsInf(val, -1):
		fb.writeJSONString("-Inf")
	default:
		var tmp [40]byte
		fb.writeRaw(strconv.AppendFloat(tmp[:0], val, 'f', -1, bits))
	}
}
