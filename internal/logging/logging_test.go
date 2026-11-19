package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestJSONFormatAndLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, slog.LevelWarn, "json")
	log.Info("не должно попасть")
	log.Warn("важное", "k", "v")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("не JSON: %v: %q", err, buf.String())
	}
	if rec["msg"] != "важное" || rec["k"] != "v" || rec["level"] != "WARN" {
		t.Fatalf("неожиданная запись: %v", rec)
	}
}

func TestTextFormat(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, "text").Info("привет", "k", "v")
	if !strings.Contains(buf.String(), "msg=привет") || !strings.Contains(buf.String(), "k=v") {
		t.Fatalf("неожиданный текст: %q", buf.String())
	}
}
