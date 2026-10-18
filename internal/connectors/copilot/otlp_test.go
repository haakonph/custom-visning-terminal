package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	logs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	traces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func attribute(k string, v any) any {
	value := map[string]any{}
	switch v := v.(type) {
	case string:
		value["stringValue"] = v
	case int:
		value["intValue"] = strconv.Itoa(v)
	default:
		value["doubleValue"] = v
	}
	return map[string]any{"key": k, "value": value}
}
func otlpFixture(signal string) map[string]any {
	attrs := []any{attribute("gen_ai.conversation.id", "s1"), attribute("gen_ai.request.model", "test-model"), attribute("client.name", "copilot-cli")}
	var field, scopeField string
	var scope map[string]any
	switch signal {
	case "traces":
		field, scopeField = "resourceSpans", "scopeSpans"
		span := map[string]any{
			"name": "chat", "traceId": "AQEBAQEBAQEBAQEBAQEBAQ==", "spanId": "AgICAgICAgI=",
			"startTimeUnixNano": "1792231200000000000", "endTimeUnixNano": "1792231200100000000",
			"attributes": []any{attribute("gen_ai.usage.input_tokens", 12), attribute("gen_ai.usage.output_tokens", 3), attribute("gen_ai.input.messages", `[{"role":"user","content":"fixture-prompt"}]`), attribute("gen_ai.output.messages", `[{"role":"assistant","content":[{"type":"text","text":"fixture-response"}]}]`)},
			"events":     []any{map[string]any{"name": "tool.complete", "timeUnixNano": "1792231200050000000", "attributes": []any{attribute("gen_ai.tool.name", "shell"), attribute("gen_ai.tool.call.id", "c1"), attribute("gen_ai.tool.call.result", "fixture-result")}}},
		}
		scope = map[string]any{"spans": []any{span}}
	case "logs":
		field, scopeField = "resourceLogs", "scopeLogs"
		scope = map[string]any{"logRecords": []any{map[string]any{"eventName": "gen_ai.assistant.message", "timeUnixNano": "1792231200000000000", "body": map[string]any{"stringValue": "fixture-log"}, "attributes": []any{attribute("event.id", "log-1")}}}}
	default:
		field, scopeField = "resourceMetrics", "scopeMetrics"
		point := map[string]any{"timeUnixNano": "1792231200000000000", "asInt": "9", "attributes": []any{attribute("gen_ai.token.type", "input")}}
		metric := map[string]any{"name": "gen_ai.client.token.usage", "sum": map[string]any{"aggregationTemporality": 2, "dataPoints": []any{point}}}
		scope = map[string]any{"metrics": []any{metric}}
	}
	return map[string]any{field: []any{map[string]any{"resource": map[string]any{"attributes": attrs}, scopeField: []any{scope}}}}
}
func TestOTLPJSONAndGeneratedProtobufForAllSignals(t *testing.T) {
	for _, signal := range []string{"traces", "logs", "metrics"} {
		for _, kind := range []string{"application/json", "application/x-protobuf"} {
			t.Run(signal+"/"+kind, func(t *testing.T) {
				b, _ := json.Marshal(otlpFixture(signal))
				if kind == "application/x-protobuf" {
					var message proto.Message
					switch signal {
					case "traces":
						message = &traces.ExportTraceServiceRequest{}
					case "logs":
						message = &logs.ExportLogsServiceRequest{}
					case "metrics":
						message = &metrics.ExportMetricsServiceRequest{}
					}
					if err := protojson.Unmarshal(b, message); err != nil {
						t.Fatal(err)
					}
					var err error
					b, err = proto.Marshal(message)
					if err != nil {
						t.Fatal(err)
					}
				}
				s := service(t, testOptions(t))
				req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4318/v1/"+signal, bytes.NewReader(b))
				req.Header.Set("Content-Type", kind)
				response := httptest.NewRecorder()
				s.handler().ServeHTTP(response, req)
				if response.Code != 200 {
					t.Fatalf("OTLP rejected: %d %s", response.Code, response.Body.String())
				}
				// Retrying an export must not add usage or duplicate its timeline.
				req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4318/v1/"+signal, bytes.NewReader(b))
				req.Header.Set("Content-Type", kind)
				s.handler().ServeHTTP(httptest.NewRecorder(), req)
				rows, err := s.Sessions(context.Background())
				if err != nil || len(rows) != 1 || rows[0].ID != "s1" || rows[0].Model != "test-model" {
					t.Fatalf("session: %+v %v", rows, err)
				}
				d, _ := s.Detail(context.Background(), "s1")
				switch signal {
				case "traces":
					if len(d.Events) != 4 || rows[0].InputTokens == nil || *rows[0].InputTokens != 12 || rows[0].OutputTokens == nil || *rows[0].OutputTokens != 3 || rows[0].DurationMs == nil || *rows[0].DurationMs != 100 || len(rows[0].TraceIDs) != 1 || rows[0].TraceIDs[0] != strings.Repeat("01", 16) {
						t.Fatalf("trace data: %+v %+v", rows[0], d.Events)
					}
					var prompt, response, result bool
					for _, e := range d.Events {
						prompt = prompt || e.Content == "fixture-prompt"
						response = response || e.Content == "fixture-response"
						result = result || e.Result == "fixture-result"
					}
					if !prompt || !response || !result {
						t.Fatal("messages/tool result lost")
					}
				case "logs":
					if len(d.Events) != 1 || d.Events[0].Content != "fixture-log" || d.Events[0].Role != "assistant" {
						t.Fatalf("logs: %+v", d)
					}
				case "metrics":
					if len(d.Events) != 1 || rows[0].InputTokens == nil || *rows[0].InputTokens != 9 {
						t.Fatalf("metrics: %+v %+v", rows, d)
					}
				}
				reloaded := service(t, s.options)
				history, _ := reloaded.Detail(context.Background(), "s1")
				if len(history.Events) != len(d.Events) {
					t.Fatal("OTLP history was not persisted")
				}
			})
		}
	}
}
func TestReceiverGuardsAndPayloadLimits(t *testing.T) {
	s := service(t, testOptions(t))
	for _, test := range []struct {
		name, method, path, kind, body, host, origin, encoding string
		want                                                   int
	}{
		{name: "external host", method: "POST", path: "/v1/logs", kind: "application/json", body: `{"resourceLogs":[]}`, host: "attacker.test", want: 403},
		{name: "browser origin", method: "POST", path: "/v1/logs", kind: "application/json", body: `{"resourceLogs":[]}`, origin: "https://attacker.test", want: 403},
		{name: "wrong method", method: "GET", path: "/v1/logs", want: 405},
		{name: "wrong path", method: "POST", path: "/api/copilot/sessions", want: 404},
		{name: "bad JSON", method: "POST", path: "/v1/logs", kind: "application/json", body: "private malformed content", want: 400},
		{name: "wrong shape", method: "POST", path: "/v1/logs", kind: "application/json", body: `{"resourceLogs":{}}`, want: 400},
		{name: "bad protobuf", method: "POST", path: "/v1/traces", kind: "application/x-protobuf", body: "invalid", want: 400},
		{name: "compressed", method: "POST", path: "/v1/logs", kind: "application/json", encoding: "gzip", want: 415},
		{name: "large body", method: "POST", path: "/v1/logs", kind: "application/json", body: strings.Repeat("x", (4<<20)+1), want: 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "http://127.0.0.1:4318"+test.path, strings.NewReader(test.body))
			req.Header.Set("Content-Type", test.kind)
			req.Header.Set("Origin", test.origin)
			req.Header.Set("Content-Encoding", test.encoding)
			if test.host != "" {
				req.Host = test.host
			}
			w := httptest.NewRecorder()
			s.handler().ServeHTTP(w, req)
			if w.Code != test.want || strings.Contains(w.Body.String(), "private malformed") {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
		})
	}
	rows, _ := s.Sessions(context.Background())
	if len(rows) != 0 {
		t.Fatal("invalid request mutated history")
	}
}
func TestRealReceiverLifecycleAndPortConflict(t *testing.T) {
	ctx := context.Background()
	s := service(t, testOptions(t))
	s.Start(ctx)
	t.Cleanup(func() { _ = s.Close() })
	d, _ := s.Diagnostics(ctx)
	if !d.ReceiverRunning || d.Port == 0 {
		t.Fatalf("receiver not listening: %+v", d)
	}
	b, _ := json.Marshal(otlpFixture("logs"))
	response, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/v1/logs", d.Port), "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Diagnostics(ctx)
	if d.ReceiverRunning {
		t.Fatal("receiver remained running")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	o := testOptions(t)
	o.Port = listener.Addr().(*net.TCPAddr).Port
	write(t, o.SessionRoot+"/s1/events.jsonl", eventJSON("1", "user.message", map[string]any{"content": "local-still-works"}))
	conflict := service(t, o)
	conflict.Start(ctx)
	conflict.Scan(ctx)
	defer conflict.Close()
	d, _ = conflict.Diagnostics(ctx)
	rows, _ := conflict.Sessions(ctx)
	if d.ReceiverRunning || len(d.Errors) == 0 || len(rows) != 1 {
		t.Fatalf("port conflict broke local import: %+v %+v", d, rows)
	}
	o = testOptions(t)
	o.Enabled = false
	disabled := service(t, o)
	disabled.Start(ctx)
	defer disabled.Close()
	d, _ = disabled.Diagnostics(ctx)
	if d.ReceiverRunning {
		t.Fatal("disabled service opened receiver")
	}
	req := httptest.NewRequest("POST", "http://127.0.0.1:4318/v1/logs", nil)
	w := httptest.NewRecorder()
	disabled.handler().ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatal("disabled receiver accepted request")
	}
}
