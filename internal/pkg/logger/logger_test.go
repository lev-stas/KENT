package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestNewAddsExporterLogRecordType(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	log := newWithWriter("info", output)

	log.Info(context.Background(), "collector started", "component", "collector")

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("logger output is not valid JSON: %v", err)
	}

	if got["recordType"] != "exporter_log" {
		t.Fatalf("unexpected recordType: %v", got["recordType"])
	}
	if got["msg"] != "collector started" {
		t.Fatalf("unexpected msg: %v", got["msg"])
	}
	if got["component"] != "collector" {
		t.Fatalf("unexpected component: %v", got["component"])
	}
}
