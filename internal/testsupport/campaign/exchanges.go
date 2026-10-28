package campaign

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const exchangesFile = "worker-exchanges.jsonl"

type exchange struct {
	Kind     string `json:"kind"`
	SentAt   string `json:"sent_at"`
	EndedAt  string `json:"ended_at"`
	Request  any    `json:"request"`
	Response []any  `json:"response"`
	Error    string `json:"error,omitempty"`
}

type workerCall struct {
	recording *Recording
	kind      string
	sentAt    time.Time
	request   proto.Message
	events    []proto.Message
}

func (w *Worker) call(kind string, request proto.Message) *workerCall {
	call := &workerCall{kind: kind, sentAt: time.Now(), request: request}
	if w.LogExchanges {
		call.recording = w.Recording
	}
	return call
}

func (c *workerCall) passing(emit func(*runtimev1.EpisodeEvent) error) func(*runtimev1.EpisodeEvent) error {
	return func(event *runtimev1.EpisodeEvent) error {
		c.heard(event)
		return emit(event)
	}
}

func (c *workerCall) heard(event proto.Message) { c.events = append(c.events, event) }

func (c *workerCall) end(callErr error) error {
	if c.recording == nil {
		return callErr
	}
	line, err := c.exchangeLine(callErr)
	if err == nil {
		err = c.recording.appendLine(exchangesFile, line)
	}
	if err != nil {
		return errors.Join(callErr, fmt.Errorf("log the %s exchange: %w", c.kind, err))
	}
	return callErr
}

func (c *workerCall) exchangeLine(callErr error) ([]byte, error) {
	request, err := readable(c.request)
	if err != nil {
		return nil, err
	}
	response, err := readableAll(c.events)
	if err != nil {
		return nil, err
	}
	line, err := json.Marshal(exchange{Kind: c.kind, SentAt: kernel.FormatTime(c.sentAt), EndedAt: kernel.FormatTime(time.Now()),
		Request: request, Response: response, Error: errorText(callErr)})
	if err != nil {
		return nil, fmt.Errorf("encode the exchange: %w", err)
	}
	return line, nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func readableAll(messages []proto.Message) ([]any, error) {
	shown := make([]any, 0, len(messages))
	for _, message := range messages {
		one, err := readable(message)
		if err != nil {
			return nil, err
		}
		shown = append(shown, one)
	}
	return shown, nil
}

func readable(message proto.Message) (any, error) {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode the wire message: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("decode the wire message: %w", err)
	}
	delete(fields, "capability_token")
	return embedJSON("", fields), nil
}

func embedJSON(name string, value any) any {
	switch shown := value.(type) {
	case map[string]any:
		for key, inner := range shown {
			shown[key] = embedJSON(key, inner)
		}
	case []any:
		for i, inner := range shown {
			shown[i] = embedJSON(name, inner)
		}
	case string:
		return embeddedDocument(name, shown)
	}
	return value
}

func embeddedDocument(name, text string) any {
	if !strings.HasSuffix(name, "_json") {
		return text
	}
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil || !json.Valid(data) {
		return text
	}
	return json.RawMessage(data)
}
