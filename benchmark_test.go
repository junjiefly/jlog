package jlog

import (
	"io"
	"testing"
)

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (discardWriter) Flush() error                { return nil }
func (discardWriter) Close() error                { return nil }
func (discardWriter) SetOutput(bool, []io.Writer) {}

func benchmarkJSON(b *testing.B, event func() *entry) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event().
			Str("request_id", "JOSSTX0001").
			Str("remote_ip", "10.0.0.8").
			Str("source_bucket", "video-a").
			Str("operation", "REST.GET.OBJECT").
			Str("request_uri", "/video-a/a/b/c.mp4").
			Int("http_status", 200).
			Int64("bytes_sent_size", 1048576).
			Int64("total_duration", 12).
			Msg("")
	}
}

func BenchmarkJSONDefault(b *testing.B) {
	oldWriter := loggers[appLog].Writer
	loggers[appLog].Writer = discardWriter{}
	defer func() { loggers[appLog].Writer = oldWriter }()

	benchmarkJSON(b, Info)
}

func BenchmarkJSONCustom(b *testing.B) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	defer restoreCustomLogsB(b, oldLogs, oldNames, oldFiles)

	customLog, err := RegisterCustomLog(CustomLogConfig{
		Name:       "bench",
		FileSuffix: ".jsonl",
	})
	if err != nil {
		b.Fatal(err)
	}
	customLog.logger.Writer = discardWriter{}

	benchmarkJSON(b, customLog.Event)
}

func BenchmarkJSONCustomOmitTime(b *testing.B) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	defer restoreCustomLogsB(b, oldLogs, oldNames, oldFiles)

	customLog, err := RegisterCustomLog(CustomLogConfig{
		Name:       "bench-omit-time",
		FileSuffix: ".jsonl",
		OmitTime:   true,
	})
	if err != nil {
		b.Fatal(err)
	}
	customLog.logger.Writer = discardWriter{}

	benchmarkJSON(b, customLog.Event)
}

func BenchmarkJSONCustomWriterFactory(b *testing.B) {
	oldLogs, oldNames, oldFiles := saveCustomLogs()
	defer restoreCustomLogsB(b, oldLogs, oldNames, oldFiles)

	customLog, err := RegisterCustomLog(CustomLogConfig{
		Name:       "bench-writer-factory",
		FileSuffix: ".jsonl",
		OmitTime:   true,
		WriterFactory: func() (CustomWriter, error) {
			return discardWriter{}, nil
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	customLog.logger.create()

	benchmarkJSON(b, customLog.Event)
}

func restoreCustomLogsB(b *testing.B, logs []*CustomLog, names, files map[string]struct{}) {
	b.Helper()
	customLogMu.Lock()
	customLogs = logs
	customLogNames = names
	customLogFiles = files
	customLogMu.Unlock()
}
