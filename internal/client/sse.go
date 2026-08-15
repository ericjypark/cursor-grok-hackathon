// Package client speaks to the field-note backend's /preprocess SSE endpoint.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Request mirrors the backend's PreprocessRequest. Only Website is required.
type Request struct {
	Website string `json:"website"`
	Name    string `json:"name,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Form    string `json:"form,omitempty"`
}

// Event is one decoded server-sent event.
type Event struct {
	Kind string // "stage", "error" or "result"
	Data []byte
}

func (e Event) Stage() (StageEvent, error) {
	var v StageEvent
	return v, json.Unmarshal(e.Data, &v)
}

func (e Event) Err() (ErrorEvent, error) {
	var v ErrorEvent
	return v, json.Unmarshal(e.Data, &v)
}

func (e Event) Result() (ProductDossier, error) {
	var v ResultEvent
	if err := json.Unmarshal(e.Data, &v); err != nil {
		return ProductDossier{}, err
	}
	return v.Dossier, nil
}

// A finished dossier runs well past bufio's 64KB default line limit, so the
// scanner gets a much larger ceiling.
const maxEventBytes = 8 << 20

// Parse decodes an SSE stream, invoking fn once per event. Pure: no network,
// which is what makes it testable.
func Parse(r io.Reader, fn func(Event) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxEventBytes)

	kind := ""
	var data []string

	flush := func() error {
		if len(data) == 0 {
			kind = ""
			return nil
		}
		ev := Event{Kind: kind, Data: []byte(strings.Join(data, "\n"))}
		kind, data = "", nil
		return fn(ev)
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"): // keepalive comment
		case strings.HasPrefix(line, "event:"):
			kind = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return flush()
}

// Stream posts the request and delivers decoded events on the returned channel,
// which closes when the run ends. Transport failures arrive as a fatal
// ErrorEvent so callers have exactly one failure path to render.
func Stream(ctx context.Context, baseURL string, req Request) (<-chan Event, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(baseURL, "/") + "/preprocess"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cannot reach backend at %s: %w", baseURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("backend returned %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}

	out := make(chan Event)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		err := Parse(resp.Body, func(ev Event) error {
			select {
			case out <- ev:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil && ctx.Err() == nil {
			payload, _ := json.Marshal(ErrorEvent{Detail: err.Error(), Fatal: ptr(true)})
			select {
			case out <- Event{Kind: "error", Data: payload}:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

func ptr[T any](v T) *T { return &v }
