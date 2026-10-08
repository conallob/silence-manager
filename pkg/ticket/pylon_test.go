package ticket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pylonServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request, body map[string]any)) *PylonTicketSystem {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Expected bearer auth, got %q", got)
		}
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		h(w, r, body)
	}))
	t.Cleanup(srv.Close)
	return NewPylonTicketSystem(srv.URL+"/", "tok", "acct-1", "", "")
}

func TestNewPylonTicketSystem_Defaults(t *testing.T) {
	p := NewPylonTicketSystem("", "tok", "acct", "", "")
	if p.baseURL != DefaultPylonAPIURL || p.annotationPrefix != "silence-manager" {
		t.Errorf("Unexpected defaults: %+v", p)
	}
}

func TestPylonGetTicket(t *testing.T) {
	p := pylonServer(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		if r.Method != http.MethodGet || r.URL.Path != "/issues/42" {
			t.Errorf("Unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"id":"abc","number":42,"title":"Disk","state":"waiting_on_you",
			"body_html":"<p>silence-manager: sil-9</p><p>Some &amp; details</p>","created_at":"2026-01-02T03:04:05Z",
			"tags":["alert"],"assignee":{"email":"a@b.c"}}}`))
	})
	tkt, err := p.GetTicket("42")
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if tkt.Key != "42" || tkt.SilenceRef != "sil-9" || tkt.Status != StatusInProgress {
		t.Errorf("Unexpected ticket: %+v", tkt)
	}
	if !strings.Contains(tkt.Description, "Some & details") || tkt.Assignee != "a@b.c" {
		t.Errorf("Unexpected description/assignee: %+v", tkt)
	}
	if !p.IsOpen(tkt) {
		t.Error("Expected ticket to be open")
	}
}

func TestPylonGetTicket_NotFound(t *testing.T) {
	p := pylonServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := p.GetTicket("1"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected not found, got %v", err)
	}
}

func TestPylonCreateTicket(t *testing.T) {
	p := pylonServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		if r.Method != http.MethodPost || r.URL.Path != "/issues" {
			t.Errorf("Unexpected request %s %s", r.Method, r.URL.Path)
		}
		if body["account_id"] != "acct-1" || body["title"] != "Alert" {
			t.Errorf("Unexpected body: %v", body)
		}
		if body["body_html"] != "<p>silence-manager: s1</p><p>a &lt;b&gt;</p>" {
			t.Errorf("Unexpected body_html: %v", body["body_html"])
		}
		_, _ = w.Write([]byte(`{"data":{"id":"abc","number":7}}`))
	})
	key, err := p.CreateTicket(&Ticket{Summary: "Alert", Description: "a <b>", SilenceRef: "s1"})
	if err != nil || key != "7" {
		t.Errorf("CreateTicket = %q, %v", key, err)
	}
}

func TestPylonCloseAndReopen(t *testing.T) {
	var calls []string
	p := pylonServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPatch {
			calls = append(calls, body["state"].(string))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := p.CloseTicket("5", "bye"); err != nil {
		t.Fatalf("CloseTicket: %v", err)
	}
	if err := p.ReopenTicket("5", ""); err != nil {
		t.Fatalf("ReopenTicket: %v", err)
	}
	want := "POST /issues/5/note|PATCH /issues/5|closed|PATCH /issues/5|new"
	if got := strings.Join(calls, "|"); got != want {
		t.Errorf("calls = %s, want %s", got, want)
	}
}

func TestPylonMapState(t *testing.T) {
	p := NewPylonTicketSystem("", "t", "a", "", "")
	tests := map[string]TicketStatus{
		"new": StatusOpen, "custom": StatusOpen, "on_hold": StatusInProgress,
		"waiting_on_customer": StatusInProgress, "closed": StatusClosed,
	}
	for in, want := range tests {
		if got := p.mapPylonState(in); got != want {
			t.Errorf("mapPylonState(%q) = %s, want %s", in, got, want)
		}
	}
	if p.IsResolved(&Ticket{Status: StatusClosed}) || !p.IsClosed(&Ticket{Status: StatusClosed}) {
		t.Error("Closed ticket should be closed but not resolved")
	}
}

func TestPylonErrorStatus(t *testing.T) {
	p := pylonServer(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		w.WriteHeader(http.StatusForbidden)
	})
	if err := p.AddComment("1", "x"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("Expected 403 error, got %v", err)
	}
}
