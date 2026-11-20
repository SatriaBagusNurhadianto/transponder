package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestNewLoggerFiltersLowerLevels(t *testing.T) {
	tests := []struct {
		name, level string
		want        []string
	}{
		{name: "debug", level: "debug", want: []string{"DEBUG", "INFO", "WARN", "ERROR"}},
		{name: "info", level: "info", want: []string{"INFO", "WARN", "ERROR"}},
		{name: "warn", level: "WaRn", want: []string{"WARN", "ERROR"}},
		{name: "error", level: "ERROR", want: []string{"ERROR"}},
		{name: "empty means info", level: "", want: []string{"INFO", "WARN", "ERROR"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := NewLogger(&buf, tt.level)
			if err != nil {
				t.Fatal(err)
			}
			logger.Debug("d")
			logger.Info("i")
			logger.Warn("w")
			logger.Error("e")
			got := decodeLevels(t, &buf)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("levels = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewLoggerBadLevel(t *testing.T) {
	_, err := NewLogger(io.Discard, "loud")
	if err == nil || !strings.Contains(err.Error(), "loud") {
		t.Fatalf("err = %v, want it to name the level", err)
	}
}

func TestNewLoggerJSON(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(&buf, "info")
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hello")
	rec := decodeRecord(t, buf.Bytes())
	if rec["service"] != "transponder" || rec["level"] != "INFO" || rec["msg"] != "hello" {
		t.Fatalf("record = %#v", rec)
	}
	stamp, ok := rec["time"].(string)
	if !ok {
		t.Fatalf("time = %#v", rec["time"])
	}
	if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		t.Fatalf("time %q: %v", stamp, err)
	}
}

func decodeLevels(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	var levels []string
	sc := bufio.NewScanner(buf)
	for sc.Scan() {
		rec := decodeRecord(t, sc.Bytes())
		if rec["service"] != "transponder" {
			t.Fatalf("service = %#v", rec["service"])
		}
		level, ok := rec["level"].(string)
		if !ok {
			t.Fatalf("level = %#v", rec["level"])
		}
		levels = append(levels, level)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return levels
}

func decodeRecord(t *testing.T, line []byte) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil {
		t.Fatalf("line %q: %v", line, err)
	}
	for _, key := range []string{"service", "level", "msg", "time"} {
		if _, ok := rec[key]; !ok {
			t.Fatalf("record %#v missing %s", rec, key)
		}
	}
	return rec
}
