package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collect(t *testing.T, body string) []Event {
	t.Helper()
	var got []Event
	if err := Parse(strings.NewReader(body), func(e Event) error {
		got = append(got, e)
		return nil
	}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return got
}

func TestParseSplitsEventsOnBlankLines(t *testing.T) {
	got := collect(t, "event: stage\ndata: {\"stage\":\"map\"}\n\nevent: result\ndata: {\"a\":1}\n\n")

	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	if got[0].Kind != "stage" || string(got[0].Data) != `{"stage":"map"}` {
		t.Errorf("first event wrong: %+v", got[0])
	}
	if got[1].Kind != "result" {
		t.Errorf("second event wrong: %+v", got[1])
	}
}

func TestParseIgnoresKeepaliveComments(t *testing.T) {
	got := collect(t, ": ping\n\nevent: stage\ndata: {}\n\n")

	if len(got) != 1 || got[0].Kind != "stage" {
		t.Fatalf("keepalive leaked into events: %+v", got)
	}
}

func TestParseJoinsMultilineData(t *testing.T) {
	got := collect(t, "event: result\ndata: {\"a\":\ndata: 1}\n\n")

	if string(got[0].Data) != "{\"a\":\n1}" {
		t.Errorf("multiline data not joined: %q", got[0].Data)
	}
}

func TestParseHandlesFinalEventWithoutTrailingBlankLine(t *testing.T) {
	got := collect(t, "event: stage\ndata: {}")

	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
}

// A real dossier blows past bufio.Scanner's 64KB default line limit, which
// would silently truncate the run's only meaningful payload.
func TestParseHandlesPayloadsLargerThanTheDefaultScannerBuffer(t *testing.T) {
	big := strings.Repeat("x", 300_000)
	body := fmt.Sprintf("event: result\ndata: {\"blob\":%q}\n\n", big)

	got := collect(t, body)

	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	var parsed struct {
		Blob string `json:"blob"`
	}
	if err := json.Unmarshal(got[0].Data, &parsed); err != nil {
		t.Fatalf("large payload did not survive: %v", err)
	}
	if len(parsed.Blob) != len(big) {
		t.Errorf("payload truncated: got %d bytes, want %d", len(parsed.Blob), len(big))
	}
}

const dossierJSON = `{"identity":{"canonical_name":"Acme","slug":"acme"},
	"what":{},"vocabulary":{},"disambiguation":{"ambiguity_score":0.5},
	"provenance":{"generated_at":"2026-08-15T00:00:00Z","runtime_ms":12}}`

func TestEventDecodersMapOntoGeneratedTypes(t *testing.T) {
	ev := Event{Kind: "result", Data: []byte(`{"note":{"dossier":` + dossierJSON + `}}`)}

	n, err := ev.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if n.Dossier.Identity.CanonicalName != "Acme" || n.Dossier.Disambiguation.AmbiguityScore != 0.5 {
		t.Errorf("decoded wrong: %+v", n.Dossier.Identity)
	}
	if n.Harvest != nil {
		t.Errorf("a t0-only run should carry no harvest, got %+v", n.Harvest)
	}
}

// T0 publishes its dossier before T1 starts, so the CLI has something to show
// during the minutes the scrape takes.
func TestDossierEventDecodesOnItsOwn(t *testing.T) {
	ev := Event{Kind: "dossier", Data: []byte(`{"dossier":` + dossierJSON + `}`)}

	d, err := ev.Dossier()
	if err != nil {
		t.Fatalf("Dossier: %v", err)
	}
	if d.Identity.CanonicalName != "Acme" {
		t.Errorf("decoded wrong: %+v", d.Identity)
	}
}

func TestHarvestEventReportsWhetherThePostsAreLive(t *testing.T) {
	ev := Event{Kind: "harvest", Data: []byte(`{"harvest":{"targets":{},"live":false,
	"source_note":"recorded signals","posts":[{"id":"post_rd_1","source":"reddit",
	"url":"https://reddit.com/r/x/comments/1/","score":412}]}}`)}

	h, err := ev.Harvest()
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if h.Live {
		t.Error("a fallback harvest must not claim to be live")
	}
	if h.Posts == nil || len(*h.Posts) != 1 || (*h.Posts)[0].Source != "reddit" {
		t.Errorf("posts decoded wrong: %+v", h.Posts)
	}
}

func TestStreamDeliversServerEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/preprocess" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: stage\ndata: {\"stage\":\"map\",\"status\":\"done\"}\n\n")
		fmt.Fprint(w, "event: result\ndata: {\"dossier\":{\"identity\":{\"canonical_name\":\"Acme\",\"slug\":\"acme\"},\"what\":{},\"vocabulary\":{},\"disambiguation\":{\"ambiguity_score\":0.25},\"provenance\":{\"generated_at\":\"2026-08-15T00:00:00Z\",\"runtime_ms\":1}}}\n\n")
	}))
	defer srv.Close()

	events, err := Stream(context.Background(), srv.URL, Request{Website: "https://acme.dev"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var kinds []string
	for ev := range events {
		kinds = append(kinds, ev.Kind)
	}
	if strings.Join(kinds, ",") != "stage,result" {
		t.Errorf("got events %v", kinds)
	}
}

func TestRequestOmitsABlankWebsite(t *testing.T) {
	// The repo alone is a valid run; a website key with an empty string is not.
	body, err := json.Marshal(Request{Repo: "acme/acme"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "website") {
		t.Errorf("blank website was sent: %s", body)
	}
}

func TestStreamSurfacesNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"missing required env var(s): EXA_API_KEY"}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := Stream(context.Background(), srv.URL, Request{Website: "https://acme.dev"})

	if err == nil || !strings.Contains(err.Error(), "EXA_API_KEY") {
		t.Fatalf("want the backend's reason surfaced, got %v", err)
	}
}

func TestStreamReportsUnreachableBackend(t *testing.T) {
	_, err := Stream(context.Background(), "http://127.0.0.1:1", Request{Website: "https://acme.dev"})

	if err == nil || !strings.Contains(err.Error(), "cannot reach backend") {
		t.Fatalf("want a clear connection error, got %v", err)
	}
}
