package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// appendOTLPLog runs under s.mu. Each line preserves one validated request,
// including fields that the current event normalizer does not recognize.
func (s *Service) appendOTLPLog(signal, kind string, body []byte) error {
	if s.options.OTLPLogPath == "" || !s.options.CaptureContent {
		return nil
	}
	record := struct {
		Time        string          `json:"time"`
		Signal      string          `json:"signal"`
		ContentType string          `json:"content_type"`
		Payload     json.RawMessage `json:"payload,omitempty"`
		Protobuf    []byte          `json:"protobuf_base64,omitempty"`
	}{Time: time.Now().UTC().Format(time.RFC3339Nano), Signal: signal, ContentType: kind}
	payload, err := otlpJSON(signal, kind, body)
	if err != nil {
		return err
	}
	record.Payload = payload
	if kind == "application/x-protobuf" {
		record.Protobuf = body
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.options.OTLPLogPath), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.options.OTLPLogPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	_, writeErr := f.Write(append(line, '\n'))
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
