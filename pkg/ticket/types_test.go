package ticket

import "testing"

func TestStatusPredicates(t *testing.T) {
	tests := []struct {
		status                 TicketStatus
		resolved, closed, open bool
	}{
		{StatusOpen, false, false, true},
		{StatusInProgress, false, false, true},
		{StatusResolved, true, true, false},
		{StatusClosed, false, true, false},
		{StatusReopened, false, false, false},
		{"", false, false, false},
	}
	for _, tt := range tests {
		tkt := &Ticket{Status: tt.status}
		if got := statusIsResolved(tkt); got != tt.resolved {
			t.Errorf("statusIsResolved(%q) = %v, want %v", tt.status, got, tt.resolved)
		}
		if got := statusIsClosed(tkt); got != tt.closed {
			t.Errorf("statusIsClosed(%q) = %v, want %v", tt.status, got, tt.closed)
		}
		if got := statusIsOpen(tkt); got != tt.open {
			t.Errorf("statusIsOpen(%q) = %v, want %v", tt.status, got, tt.open)
		}
	}
}

// Every implementation must answer the predicates the same way, since the
// synchronizer's behaviour depends on them.
func TestImplementationsUseSharedPredicates(t *testing.T) {
	systems := map[string]TicketSystem{
		"jira":   NewJiraTicketSystem("http://x", "u", "t", "P", ""),
		"linear": NewLinearTicketSystem("http://x", "k", "team", ""),
		"pylon":  NewPylonTicketSystem("http://x", "t", "acct", "", ""),
	}
	for name, ts := range systems {
		for _, status := range []TicketStatus{StatusOpen, StatusInProgress, StatusResolved, StatusClosed, StatusReopened} {
			tkt := &Ticket{Status: status}
			if ts.IsResolved(tkt) != statusIsResolved(tkt) || ts.IsClosed(tkt) != statusIsClosed(tkt) || ts.IsOpen(tkt) != statusIsOpen(tkt) {
				t.Errorf("%s disagrees with the shared predicates for %q", name, status)
			}
		}
	}
}
