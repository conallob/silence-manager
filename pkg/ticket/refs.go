package ticket

import (
	"fmt"
	"strings"
)

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

// withSilenceRef prepends the silence reference annotation to a description
func withSilenceRef(description, silenceRef, prefix string) string {
	if silenceRef == "" {
		return description
	}
	return fmt.Sprintf("%s: %s\n\n%s", prefix, silenceRef, description)
}
