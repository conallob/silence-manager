package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/conallob/silence-manager/pkg/config"
	"github.com/conallob/silence-manager/pkg/ticket"
)

func TestNewTicketSystem_Types(t *testing.T) {
	tests := []struct {
		name   string
		system string
		check  func(ticket.TicketSystem) bool
	}{
		{"jira", config.TicketSystemJira, func(ts ticket.TicketSystem) bool { _, ok := ts.(*ticket.JiraTicketSystem); return ok }},
		{"linear", config.TicketSystemLinear, func(ts ticket.TicketSystem) bool { _, ok := ts.(*ticket.LinearTicketSystem); return ok }},
		{"pylon", config.TicketSystemPylon, func(ts ticket.TicketSystem) bool { _, ok := ts.(*ticket.PylonTicketSystem); return ok }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := newTicketSystem(&config.Config{TicketSystem: tt.system})
			if err != nil {
				t.Fatalf("newTicketSystem() error = %v", err)
			}
			if !tt.check(ts) {
				t.Errorf("newTicketSystem(%q) returned %T", tt.system, ts)
			}
		})
	}
}

func TestNewTicketSystem_Unknown(t *testing.T) {
	for _, system := range []string{"", "trello", "Jira"} {
		ts, err := newTicketSystem(&config.Config{TicketSystem: system})
		if err == nil || ts != nil {
			t.Errorf("newTicketSystem(%q) = %v, %v; want an error and no client", system, ts, err)
		}
	}
}

// The configuration fields must reach the clients, not just select a type.
// Each case points the client at a test server and checks what it sends.

func TestNewTicketSystem_JiraWiring(t *testing.T) {
	var gotPath, gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		_, _ = w.Write([]byte(`{"key":"OPS-1","fields":{"summary":"s","description":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"custom: sil-1"}]}]},"status":{"name":"To Do"}}}`))
	}))
	defer srv.Close()

	ts, err := newTicketSystem(&config.Config{
		TicketSystem: config.TicketSystemJira,
		Jira:         config.JiraConfig{URL: srv.URL, Username: "me@example.com", APIToken: "tok", ProjectKey: "OPS"},
		Sync:         config.SyncConfig{AnnotationPrefix: "custom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tkt, err := ts.GetTicket("OPS-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/rest/api/3/issue/OPS-1" || gotUser != "me@example.com" || gotPass != "tok" {
		t.Errorf("Jira request not wired from config: path=%q user=%q pass=%q", gotPath, gotUser, gotPass)
	}
	if tkt.SilenceRef != "sil-1" {
		t.Errorf("Annotation prefix not wired: SilenceRef = %q", tkt.SilenceRef)
	}
}

func TestNewTicketSystem_LinearWiring(t *testing.T) {
	var gotAuth string
	var vars map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		vars = req.Variables
		_, _ = w.Write([]byte(`{"data":{"issueCreate":{"success":true,"issue":{"id":"u","identifier":"ENG-7"}}}}`))
	}))
	defer srv.Close()

	ts, err := newTicketSystem(&config.Config{
		TicketSystem: config.TicketSystemLinear,
		Linear:       config.LinearConfig{APIURL: srv.URL, APIKey: "lin_key", TeamID: "team-uuid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := ts.CreateTicket(&ticket.Ticket{Summary: "s", Description: "d"})
	if err != nil || key != "ENG-7" {
		t.Fatalf("CreateTicket = %q, %v", key, err)
	}
	input, _ := vars["input"].(map[string]any)
	if gotAuth != "lin_key" || input["teamId"] != "team-uuid" {
		t.Errorf("Linear request not wired from config: auth=%q input=%v", gotAuth, input)
	}
}

func TestNewTicketSystem_PylonWiring(t *testing.T) {
	var gotAuth string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"data":{"id":"iss-1","number":3}}`))
	}))
	defer srv.Close()

	ts, err := newTicketSystem(&config.Config{
		TicketSystem: config.TicketSystemPylon,
		Pylon:        config.PylonConfig{APIURL: srv.URL, APIToken: "pyl_tok", AccountID: "acct-9", RequesterEmail: "oncall@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := ts.CreateTicket(&ticket.Ticket{Summary: "s", Description: "d"})
	if err != nil || key != "iss-1" {
		t.Fatalf("CreateTicket = %q, %v", key, err)
	}
	if gotAuth != "Bearer pyl_tok" || body["account_id"] != "acct-9" || body["requester_email"] != "oncall@example.com" {
		t.Errorf("Pylon request not wired from config: auth=%q body=%v", gotAuth, body)
	}
	if !strings.Contains(body["body_html"].(string), "<p>") {
		t.Errorf("Expected an HTML body, got %v", body["body_html"])
	}
}
