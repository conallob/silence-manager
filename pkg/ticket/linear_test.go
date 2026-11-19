package ticket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// linearServer routes GraphQL requests by a substring of the query
func linearServer(t *testing.T, handlers map[string]func(vars map[string]any) string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "lin_api_key" {
			t.Errorf("Expected raw API key in Authorization header, got %q", got)
		}
		var req linearGraphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("bad request body: %v", err)
		}
		for substr, h := range handlers {
			if strings.Contains(req.Query, substr) {
				seen = append(seen, substr)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(h(req.Variables)))
				return
			}
		}
		t.Errorf("unhandled query: %s", req.Query)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestNewLinearTicketSystem_Defaults(t *testing.T) {
	l := NewLinearTicketSystem("", "key", "team", "")
	if l.apiURL != DefaultLinearAPIURL {
		t.Errorf("Expected default API URL, got %q", l.apiURL)
	}
	if l.annotationPrefix != "silence-manager" {
		t.Errorf("Expected default prefix, got %q", l.annotationPrefix)
	}
}

func TestLinearGetTicket(t *testing.T) {
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"issue(id: $id)": func(vars map[string]any) string {
			if vars["id"] != "ENG-12" {
				t.Errorf("Expected id ENG-12, got %v", vars["id"])
			}
			return `{"data":{"issue":{"id":"uuid-1","identifier":"ENG-12","title":"Disk full",
				"description":"silence-manager: abc-123\n\nDetails","createdAt":"2026-01-02T03:04:05Z",
				"updatedAt":"2026-01-03T03:04:05Z","state":{"id":"s1","name":"In Progress","type":"started"},
				"assignee":{"name":"Ada"},"labels":{"nodes":[{"name":"alert"}]}}}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "silence-manager")

	tkt, err := l.GetTicket("ENG-12")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if tkt.Key != "ENG-12" || tkt.Summary != "Disk full" || tkt.SilenceRef != "abc-123" {
		t.Errorf("Unexpected ticket: %+v", tkt)
	}
	if tkt.Status != StatusInProgress || !l.IsOpen(tkt) {
		t.Errorf("Expected in-progress/open, got %s", tkt.Status)
	}
	if tkt.Assignee != "Ada" || len(tkt.Labels) != 1 || tkt.CreatedAt.IsZero() {
		t.Errorf("Unexpected metadata: %+v", tkt)
	}
}

func TestLinearGetTicket_NotFound(t *testing.T) {
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"issue(id: $id)": func(map[string]any) string {
			return `{"data":{"issue":null}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "")
	if _, err := l.GetTicket("ENG-1"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected not found error, got %v", err)
	}
}

func TestLinearGraphQLError(t *testing.T) {
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"issue(id: $id)": func(map[string]any) string {
			return `{"errors":[{"message":"Entity not found: Issue"}]}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "")
	if _, err := l.GetTicket("ENG-1"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected not found error, got %v", err)
	}
}

func TestLinearMapStateType(t *testing.T) {
	l := NewLinearTicketSystem("", "k", "t", "")
	tests := map[string]TicketStatus{
		"triage": StatusOpen, "backlog": StatusOpen, "unstarted": StatusOpen,
		"started": StatusInProgress, "completed": StatusResolved, "canceled": StatusResolved, "duplicate": StatusResolved,
	}
	for in, want := range tests {
		if got := l.mapLinearStateType(in); got != want {
			t.Errorf("mapLinearStateType(%q) = %s, want %s", in, got, want)
		}
	}
	if !l.IsResolved(&Ticket{Status: StatusResolved}) || !l.IsClosed(&Ticket{Status: StatusResolved}) {
		t.Error("Resolved tickets should be resolved and closed")
	}
}

func TestLinearCreateTicket(t *testing.T) {
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"issueCreate": func(vars map[string]any) string {
			input := vars["input"].(map[string]any)
			if input["teamId"] != "team-uuid" || input["title"] != "Alert" {
				t.Errorf("Unexpected input: %v", input)
			}
			if input["description"] != "silence-manager: sil-1\n\nbody" {
				t.Errorf("Unexpected description: %q", input["description"])
			}
			return `{"data":{"issueCreate":{"success":true,"issue":{"id":"u","identifier":"ENG-99"}}}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team-uuid", "")
	key, err := l.CreateTicket(&Ticket{Summary: "Alert", Description: "body", SilenceRef: "sil-1"})
	if err != nil || key != "ENG-99" {
		t.Errorf("CreateTicket = %q, %v", key, err)
	}
}

func TestLinearCloseAndReopen(t *testing.T) {
	states := `{"data":{"issue":{"team":{"id":"t","states":{"nodes":[
		{"id":"backlog","name":"Backlog","type":"backlog","position":0},
		{"id":"todo","name":"Todo","type":"unstarted","position":1},
		{"id":"todo2","name":"Later","type":"unstarted","position":5},
		{"id":"done","name":"Done","type":"completed","position":2},
		{"id":"cancel","name":"Canceled","type":"canceled","position":3}]}}}}}`
	var updated []string
	srv, seen := linearServer(t, map[string]func(map[string]any) string{
		"states {": func(map[string]any) string { return states },
		"commentCreate": func(vars map[string]any) string {
			if vars["input"].(map[string]any)["body"] != "note" {
				t.Errorf("Unexpected comment: %v", vars)
			}
			return `{"data":{"commentCreate":{"success":true}}}`
		},
		"issueUpdate": func(vars map[string]any) string {
			updated = append(updated, vars["input"].(map[string]any)["stateId"].(string))
			return `{"data":{"issueUpdate":{"success":true}}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "")

	if err := l.CloseTicket("ENG-1", "note"); err != nil {
		t.Fatalf("CloseTicket: %v", err)
	}
	if err := l.ReopenTicket("ENG-1", ""); err != nil {
		t.Fatalf("ReopenTicket: %v", err)
	}
	if len(updated) != 2 || updated[0] != "done" || updated[1] != "todo" {
		t.Errorf("Expected states [done todo], got %v", updated)
	}
	// State changes come first so a failure never leaves an orphaned comment
	if got := strings.Join((*seen)[:3], ","); got != "states {,issueUpdate,commentCreate" {
		t.Errorf("Expected state change before comment, got %v", *seen)
	}
}

func TestLinearFindState_Missing(t *testing.T) {
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"states {": func(map[string]any) string {
			return `{"data":{"issue":{"team":{"id":"t","states":{"nodes":[]}}}}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "")
	if err := l.CloseTicket("ENG-1", ""); err == nil {
		t.Error("Expected error when no completed state exists")
	}
}

func TestLinearHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()
	l := NewLinearTicketSystem(srv.URL, "bad", "team", "")
	if err := l.AddComment("ENG-1", "x"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("Expected 401 error, got %v", err)
	}
}

func TestLinearGraphQLError_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Variable \"$id\" is invalid"}]}`))
	}))
	defer srv.Close()
	l := NewLinearTicketSystem(srv.URL, "k", "team", "")
	err := l.AddComment("ENG-1", "x")
	if err == nil || !strings.Contains(err.Error(), "is invalid") {
		t.Errorf("Expected GraphQL error message to surface, got %v", err)
	}
}

func TestLinearFindState_Fallbacks(t *testing.T) {
	// No completed or unstarted states: fall back to canceled and backlog
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"states {": func(map[string]any) string {
			return `{"data":{"issue":{"team":{"id":"t","states":{"nodes":[
				{"id":"b","name":"Backlog","type":"backlog","position":0},
				{"id":"c","name":"Canceled","type":"canceled","position":1}]}}}}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "")
	if s, err := l.findState("ENG-1", "completed", "canceled"); err != nil || s.ID != "c" {
		t.Errorf("Expected canceled fallback, got %+v, %v", s, err)
	}
	if s, err := l.findState("ENG-1", "unstarted", "backlog"); err != nil || s.ID != "b" {
		t.Errorf("Expected backlog fallback, got %+v, %v", s, err)
	}
}

func TestLinearClose_StateFailureSkipsComment(t *testing.T) {
	commented := false
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"states {": func(map[string]any) string {
			return `{"data":{"issue":{"team":{"id":"t","states":{"nodes":[{"id":"d","name":"Done","type":"completed","position":0}]}}}}}`
		},
		"issueUpdate": func(map[string]any) string { return `{"errors":[{"message":"boom"}]}` },
		"commentCreate": func(map[string]any) string {
			commented = true
			return `{"data":{"commentCreate":{"success":true}}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "")
	if err := l.CloseTicket("ENG-1", "note"); err == nil {
		t.Error("Expected error when the state change fails")
	}
	if commented {
		t.Error("Comment must not be posted when the state change fails")
	}
}

func TestLinearUpdateTicket(t *testing.T) {
	srv, _ := linearServer(t, map[string]func(map[string]any) string{
		"issueUpdate": func(vars map[string]any) string {
			input := vars["input"].(map[string]any)
			if vars["id"] != "ENG-5" || input["title"] != "New" || input["description"] != "silence-manager: s2\n\nbody" {
				t.Errorf("Unexpected update: %v", vars)
			}
			return `{"data":{"issueUpdate":{"success":true}}}`
		},
	})
	l := NewLinearTicketSystem(srv.URL, "lin_api_key", "team", "")
	if err := l.UpdateTicket(&Ticket{Key: "ENG-5", Summary: "New", Description: "body", SilenceRef: "s2"}); err != nil {
		t.Errorf("UpdateTicket: %v", err)
	}
}

func TestTruncateBody(t *testing.T) {
	short := []byte("short")
	if got := truncateBody(short); got != "short" {
		t.Errorf("Short bodies should be unchanged, got %q", got)
	}
	long := []byte(strings.Repeat("x", 5000))
	if got := truncateBody(long); len(got) > 1100 || !strings.HasSuffix(got, "(truncated)") {
		t.Errorf("Expected truncated body, got %d bytes", len(got))
	}
}

// authHeaderServer returns a server that records the Authorization header of
// the single request it receives, replying with an empty issue.
func authHeaderServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":{"commentCreate":{"success":true}}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestLinearAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name string
		make func(url string) *LinearTicketSystem
		want string
	}{
		{"personal API key is sent as-is", func(u string) *LinearTicketSystem { return NewLinearTicketSystem(u, "lin_api_abc", "team", "") }, "lin_api_abc"},
		{"API key whitespace is trimmed", func(u string) *LinearTicketSystem { return NewLinearTicketSystem(u, "lin_api_abc\n", "team", "") }, "lin_api_abc"},
		{"legacy Bearer value in the API key still passes through", func(u string) *LinearTicketSystem { return NewLinearTicketSystem(u, "Bearer tok123", "team", "") }, "Bearer tok123"},
		{"OAuth token is sent as a Bearer token", func(u string) *LinearTicketSystem { return NewLinearOAuthTicketSystem(u, "tok123", "team", "") }, "Bearer tok123"},
		{"OAuth token whitespace is trimmed", func(u string) *LinearTicketSystem { return NewLinearOAuthTicketSystem(u, " tok123\n", "team", "") }, "Bearer tok123"},
		{"OAuth token with a Bearer prefix is not double-prefixed", func(u string) *LinearTicketSystem { return NewLinearOAuthTicketSystem(u, "Bearer tok123", "team", "") }, "Bearer tok123"},
		{"the Bearer prefix is matched case-insensitively", func(u string) *LinearTicketSystem { return NewLinearOAuthTicketSystem(u, "bearer tok123", "team", "") }, "Bearer tok123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, got := authHeaderServer(t)
			if err := tt.make(srv.URL).AddComment("ENG-1", "hi"); err != nil {
				t.Fatalf("AddComment: %v", err)
			}
			if *got != tt.want {
				t.Errorf("Authorization = %q, want %q", *got, tt.want)
			}
		})
	}
}

func TestLinearOAuth_Defaults(t *testing.T) {
	l := NewLinearOAuthTicketSystem("", "tok", "team", "")
	if l.apiURL != DefaultLinearAPIURL || l.annotationPrefix != "silence-manager" || l.teamID != "team" {
		t.Errorf("Unexpected defaults: %+v", l)
	}
}

// An OAuth-authenticated client must behave exactly like an API-key one apart
// from the header: read an issue, close it (state change) and comment.
func TestLinearOAuth_FullFlow(t *testing.T) {
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		var req linearGraphQLRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch {
		case strings.Contains(req.Query, "states {"):
			_, _ = w.Write([]byte(`{"data":{"issue":{"team":{"id":"t","states":{"nodes":[{"id":"d","name":"Done","type":"completed","position":0}]}}}}}`))
		case strings.Contains(req.Query, "issueUpdate"):
			_, _ = w.Write([]byte(`{"data":{"issueUpdate":{"success":true}}}`))
		case strings.Contains(req.Query, "commentCreate"):
			_, _ = w.Write([]byte(`{"data":{"commentCreate":{"success":true}}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"issue":{"id":"u","identifier":"ENG-1","title":"t","state":{"type":"started"}}}}`))
		}
	}))
	defer srv.Close()

	l := NewLinearOAuthTicketSystem(srv.URL, "tok", "team", "")
	tkt, err := l.GetTicket("ENG-1")
	if err != nil || tkt.Key != "ENG-1" || !l.IsOpen(tkt) {
		t.Fatalf("GetTicket = %+v, %v", tkt, err)
	}
	if err := l.CloseTicket("ENG-1", "done"); err != nil {
		t.Fatalf("CloseTicket: %v", err)
	}
	if len(auths) < 4 {
		t.Fatalf("expected at least 4 requests (get, states, update, comment), got %d", len(auths))
	}
	for i, a := range auths {
		if a != "Bearer tok" {
			t.Errorf("request %d used Authorization %q, want %q", i, a, "Bearer tok")
		}
	}
}
