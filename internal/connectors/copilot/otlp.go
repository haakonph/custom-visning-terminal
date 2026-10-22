package copilot

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	logs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	traces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func (s *Service) handler() http.Handler {
	return s.options.AccessLog.Handler(s.options.Usage.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if (host != "localhost" && host != "127.0.0.1" && host != "::1") || r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host) {
			http.Error(w, "Local origin required", 403)
			return
		}
		signal := strings.TrimPrefix(r.URL.Path, "/v1/")
		if r.URL.Path != "/v1/"+signal || (signal != "traces" && signal != "logs" && signal != "metrics") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "POST required", 405)
			return
		}
		if !s.options.Enabled {
			http.Error(w, "Collection disabled", 503)
			return
		}
		if enc := r.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
			http.Error(w, "Compression unsupported", 415)
			return
		}
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || (kind != "application/json" && kind != "application/x-protobuf") {
			http.Error(w, "JSON or protobuf required", 415)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, "Payload too large or unreadable", 413)
			return
		}
		events, err := decodeOTLP(signal, kind, body)
		if err != nil {
			http.Error(w, "Invalid OTLP payload", 400)
			return
		}
		if s.load() != nil {
			http.Error(w, "Local store unavailable", 503)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ingestLocked(events)
		s.diagnostics.LastOtel = time.Now().UTC().Format(time.RFC3339Nano)
		for _, e := range events {
			if e.Content != "" || e.Args != "" || e.Result != "" {
				s.diagnostics.ContentReceived = true
			}
		}
		if err := s.saveLocked(); err != nil {
			s.reportSaveErrorLocked("OTLP", err)
			http.Error(w, "Local persistence failed", 503)
			return
		}
		if kind == "application/x-protobuf" {
			w.Header().Set("Content-Type", kind)
			w.WriteHeader(200)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		}
	}), "/v1/traces", "/v1/logs", "/v1/metrics"))
}
func decodeOTLP(signal, kind string, b []byte) ([]Event, error) {
	if kind == "application/x-protobuf" {
		var message proto.Message
		switch signal {
		case "traces":
			message = &traces.ExportTraceServiceRequest{}
		case "logs":
			message = &logs.ExportLogsServiceRequest{}
		case "metrics":
			message = &metrics.ExportMetricsServiceRequest{}
		default:
			return nil, errors.New("unknown signal")
		}
		if err := proto.Unmarshal(b, message); err != nil {
			return nil, err
		}
		var err error
		b, err = protojson.Marshal(message)
		if err != nil {
			return nil, err
		}
	}
	var payload map[string]any
	if json.Unmarshal(b, &payload) != nil {
		return nil, errors.New("invalid JSON")
	}
	return normalize(signal, payload)
}
func list(v any) []any { a, _ := v.([]any); return a }
func value(v any) any {
	m := obj(v)
	for _, k := range []string{"stringValue", "intValue", "doubleValue", "boolValue", "bytesValue"} {
		if x, ok := m[k]; ok {
			return x
		}
	}
	if x, ok := m["arrayValue"]; ok {
		out := []any{}
		for _, v := range list(obj(x)["values"]) {
			out = append(out, value(v))
		}
		return out
	}
	if x, ok := m["kvlistValue"]; ok {
		return attributes(obj(x)["values"])
	}
	return nil
}
func attributes(v any) map[string]any {
	m := map[string]any{}
	for _, item := range list(v) {
		a := obj(item)
		if key := text(a["key"]); key != "" {
			m[key] = value(a["value"])
		}
	}
	return m
}
func merge(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
func identifier(v any, n int) string {
	s := text(v)
	if b, err := hex.DecodeString(s); err == nil && len(b) == n {
		return strings.ToLower(s)
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == n {
		return hex.EncodeToString(b)
	}
	return ""
}
func first(vs ...any) any {
	for _, v := range vs {
		if v != nil {
			return v
		}
	}
	return nil
}
func otelRecord(raw, a map[string]any, kind, key string, at any) Event {
	body := value(raw["body"])
	bodyObject := obj(body)
	b, _ := json.Marshal(raw)
	e := Event{Key: key, Source: "otel", SessionID: text(a["gen_ai.conversation.id"], a["session.id"], a["copilot.session.id"]), TraceID: identifier(raw["traceId"], 16), SpanID: identifier(raw["spanId"], 8), EventID: text(a["event.id"]), Time: timestamp(at), Type: kind, Role: text(a["gen_ai.role"], bodyObject["role"]), Content: printable(first(a["gen_ai.input.messages"], a["gen_ai.output.messages"], a["gen_ai.prompt"], a["gen_ai.completion"], bodyObject["content"], body)), CallID: text(a["gen_ai.tool.call.id"]), Tool: text(a["gen_ai.tool.name"]), Args: printable(a["gen_ai.tool.call.arguments"]), Result: printable(a["gen_ai.tool.call.result"]), Error: text(a["error.type"], a["exception.message"], obj(raw["status"])["message"]), Agent: text(a["gen_ai.agent.name"], a["gen_ai.agent.id"]), Parent: text(a["gen_ai.agent.parent.id"], identifier(raw["parentSpanId"], 8)), Model: text(a["gen_ai.response.model"], a["gen_ai.request.model"]), InputTokens: tokens(a["gen_ai.usage.input_tokens"]), OutputTokens: tokens(a["gen_ai.usage.output_tokens"]), Meta: map[string]any{"cwd": text(a["process.cwd"]), "client": text(a["client_name"], a["client.name"])}, Raw: b}
	if e.Role == "" {
		switch kind {
		case "gen_ai.user.message":
			e.Role = "user"
		case "gen_ai.assistant.message":
			e.Role = "assistant"
		}
	}
	return e
}
func normalize(signal string, payload map[string]any) ([]Event, error) {
	field := map[string]string{"traces": "resourceSpans", "logs": "resourceLogs", "metrics": "resourceMetrics"}[signal]
	resources, ok := payload[field].([]any)
	if !ok {
		return nil, errors.New("invalid OTLP request")
	}
	out := []Event{}
	size := 0
	add := func(e Event) error {
		b, _ := json.Marshal(e)
		size += len(b)
		if len(out) >= 20000 || size > 16<<20 {
			return errors.New("too many or too large records")
		}
		out = append(out, e)
		return nil
	}
	messages := func(e Event, a map[string]any) error {
		for _, pair := range [][2]string{{"gen_ai.input.messages", "user"}, {"gen_ai.output.messages", "assistant"}, {"gen_ai.prompt", "user"}, {"gen_ai.completion", "assistant"}} {
			item, ok := a[pair[0]]
			if !ok {
				continue
			}
			if str, ok := item.(string); ok {
				var decoded any
				if json.Unmarshal([]byte(str), &decoded) == nil {
					item = decoded
				}
			}
			items, ok := item.([]any)
			if !ok {
				items = []any{item}
			}
			for i, item := range items {
				m := obj(item)
				content := first(m["content"], m["parts"], item)
				if blocks, ok := content.([]any); ok {
					parts := []string{}
					for _, part := range blocks {
						block := obj(part)
						if block["type"] == "text" {
							parts = append(parts, text(block["content"], block["text"], printable(part)))
						} else {
							parts = append(parts, printable(part))
						}
					}
					content = strings.Join(parts, "\n")
				}
				msg := e
				msg.Key = fmt.Sprintf("message:%s:%s:%d", e.Key, pair[0], i)
				msg.Type = "message"
				msg.Role = text(m["role"], pair[1])
				msg.Content = printable(content)
				msg.InputTokens = nil
				msg.OutputTokens = nil
				msg.DurationMs = nil
				msg.Raw, _ = json.Marshal(map[string]any{"attribute": pair[0], "message": item})
				if err := add(msg); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, item := range resources {
		resource := obj(item)
		ra := attributes(obj(resource["resource"])["attributes"])
		scopes := first(resource["scopeSpans"], resource["scopeLogs"], resource["scopeMetrics"])
		for _, item := range list(scopes) {
			scope := obj(item)
			switch signal {
			case "traces":
				for _, item := range list(scope["spans"]) {
					span := obj(item)
					a := merge(ra, attributes(span["attributes"]))
					trace := identifier(span["traceId"], 16)
					spanID := identifier(span["spanId"], 8)
					e := otelRecord(span, a, text(span["name"], "span"), "span:"+text(trace, hash(ra))+":"+text(spanID, hash(span)), first(span["endTimeUnixNano"], span["startTimeUnixNano"]))
					e.StartTime = timestamp(span["startTimeUnixNano"])
					if start, err := time.Parse(time.RFC3339Nano, e.StartTime); err == nil {
						if end, err := time.Parse(time.RFC3339Nano, e.Time); err == nil {
							duration := float64(max(time.Duration(0), end.Sub(start))) / float64(time.Millisecond)
							e.DurationMs = &duration
						}
					}
					if obj(span["status"])["code"] == float64(2) || obj(span["status"])["code"] == "STATUS_CODE_ERROR" {
						e.Error = text(e.Error, "OTel span error")
					}
					if err := messages(e, a); err != nil {
						return nil, err
					}
					e.Content = ""
					if err := add(e); err != nil {
						return nil, err
					}
					for _, item := range list(span["events"]) {
						ev := obj(item)
						ea := merge(a, attributes(ev["attributes"]))
						event := otelRecord(merge(ev, map[string]any{"traceId": span["traceId"]}), ea, text(ev["name"], "event"), "event:"+e.Key+":"+hash(ev), ev["timeUnixNano"])
						event.InputTokens = nil
						event.OutputTokens = nil
						if err := add(event); err != nil {
							return nil, err
						}
					}
				}
			case "logs":
				for _, item := range list(scope["logRecords"]) {
					log := obj(item)
					a := merge(ra, attributes(log["attributes"]))
					e := otelRecord(log, a, text(log["eventName"], a["event.name"], "log"), "log:"+text(a["event.id"], hash(map[string]any{"resource": resource["resource"], "scope": scope["scope"], "log": log})), first(log["timeUnixNano"], log["observedTimeUnixNano"]))
					if n := number(log["severityNumber"]); n != nil {
						if *n >= 17 {
							e.Error = text(e.Error, printable(value(log["body"])))
						} else if *n >= 13 {
							e.Warning = printable(value(log["body"]))
						}
					}
					if a["gen_ai.input.messages"] != nil || a["gen_ai.output.messages"] != nil {
						if err := messages(e, a); err != nil {
							return nil, err
						}
						e.Content = ""
					}
					if err := add(e); err != nil {
						return nil, err
					}
				}
			case "metrics":
				for _, item := range list(scope["metrics"]) {
					metric := obj(item)
					for _, kind := range []string{"sum", "gauge", "histogram", "exponentialHistogram", "summary"} {
						data := obj(metric[kind])
						for _, item := range list(data["dataPoints"]) {
							point := obj(item)
							a := merge(ra, attributes(point["attributes"]))
							e := otelRecord(point, a, text(metric["name"]), "metric:"+hash(map[string]any{"resource": resource["resource"], "scope": scope["scope"], "name": metric["name"], "kind": kind, "point": point}), point["timeUnixNano"])
							if metric["name"] == "gen_ai.client.token.usage" {
								total := tokens(first(point["sum"], point["asInt"], point["asDouble"]))
								if a["gen_ai.token.type"] == "input" {
									e.InputTokens = total
								}
								if a["gen_ai.token.type"] == "output" {
									e.OutputTokens = total
								}
							}
							e.MetricSeries = hash(map[string]any{"name": metric["name"], "kind": kind, "attributes": a})
							if n := number(data["aggregationTemporality"]); n != nil {
								e.Temporality = int(*n)
							} else {
								switch data["aggregationTemporality"] {
								case "AGGREGATION_TEMPORALITY_DELTA":
									e.Temporality = 1
								case "AGGREGATION_TEMPORALITY_CUMULATIVE":
									e.Temporality = 2
								}
							}
							e.Raw, _ = json.Marshal(map[string]any{"metric": metric["name"], "kind": kind, "point": point})
							if err := add(e); err != nil {
								return nil, err
							}
						}
					}
				}
			}
		}
	}
	return out, nil
}
