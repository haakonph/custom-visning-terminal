package copilot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOTLPLogPreservesPayloadAndHonorsCapture(t *testing.T) {
	for _, kind := range []string{"application/json", "application/x-protobuf"} {
		t.Run(kind, func(t *testing.T) {
			o := testOptions(t)
			o.OTLPLogPath = filepath.Join(t.TempDir(), "logs", "otlp.jsonl")
			s := service(t, o)
			body := []byte(`{"resourceLogs":[],"futureField":{"value":42}}`)
			if kind != "application/json" {
				body = nil
			}
			if err := s.appendOTLPLog("logs", kind, body); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(o.OTLPLogPath)
			if err != nil {
				t.Fatal(err)
			}
			var record struct {
				Payload  json.RawMessage `json:"payload"`
				Protobuf []byte          `json:"protobuf_base64"`
				Signal   string          `json:"signal"`
			}
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			got := []byte(record.Payload)
			if kind != "application/json" {
				got = record.Protobuf
			}
			if string(got) != string(body) || record.Signal != "logs" {
				t.Fatalf("payload changed: %s", data)
			}
			info, _ := os.Stat(o.OTLPLogPath)
			if info.Mode().Perm() != 0600 {
				t.Fatal("log must be private")
			}
			s.options.CaptureContent = false
			if err := s.appendOTLPLog("logs", kind, body); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(o.OTLPLogPath)
			if string(after) != string(data) {
				t.Fatal("capture disabled still logged content")
			}
		})
	}
}

func TestOTLPHandlerLogsAllSignals(t *testing.T) {
	for _, signal := range []string{"logs", "traces", "metrics"} {
		t.Run(signal, func(t *testing.T) {
			o := testOptions(t)
			o.OTLPLogPath = filepath.Join(t.TempDir(), "otlp.jsonl")
			s := service(t, o)
			body, err := json.Marshal(otlpFixture(signal))
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4318/v1/"+signal, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			s.handler().ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", response.Code, response.Body.String())
			}
			data, err := os.ReadFile(o.OTLPLogPath)
			if err != nil {
				t.Fatal(err)
			}
			var record struct {
				Signal  string
				Payload json.RawMessage
			}
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			if record.Signal != signal || !bytes.Equal(record.Payload, body) {
				t.Fatalf("request not preserved: %s", data)
			}
		})
	}
}
