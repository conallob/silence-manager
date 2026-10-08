package ticket

import (
	"fmt"
	"strings"
)

// maxResponseBytes bounds how much of an API response is read into memory
const maxResponseBytes = 4 << 20

// truncateBody shortens an API response body for inclusion in an error message
func truncateBody(b []byte) string {
	const max = 1024
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

// extractSilenceRef extracts the silence reference from a ticket description.
// The reference is a line of the form "prefix: silence-id"; it is normally the
// first line, but any line is accepted so that content added before it (for
// example by a ticket system's editor) does not unlink the ticket.
func extractSilenceRef(description, prefix string) string {
	marker := prefix + ": "
	for _, line := range strings.Split(description, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), marker); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// withSilenceRef prepends the silence reference annotation to a description,
// first dropping any existing annotation line so that a description read back
// from the ticket system does not accumulate duplicates on update.
func withSilenceRef(description, silenceRef, prefix string) string {
	if silenceRef == "" {
		return description
	}
	marker := prefix + ": "
	var kept []string
	for _, line := range strings.Split(description, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), marker) {
			kept = append(kept, line)
		}
	}
	return fmt.Sprintf("%s: %s\n\n%s", prefix, silenceRef, strings.TrimLeft(strings.Join(kept, "\n"), "\n"))
}
