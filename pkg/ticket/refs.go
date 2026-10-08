package ticket

import (
	"fmt"
	"strings"
)

// extractSilenceRef extracts the silence reference from a ticket description.
// The reference is expected on the first line in the form "prefix: silence-id".
func extractSilenceRef(description, prefix string) string {
	marker := prefix + ": "
	if !strings.HasPrefix(description, marker) {
		return ""
	}
	rest := description[len(marker):]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

// withSilenceRef prepends the silence reference annotation to a description
func withSilenceRef(description, silenceRef, prefix string) string {
	if silenceRef == "" {
		return description
	}
	return fmt.Sprintf("%s: %s\n\n%s", prefix, silenceRef, description)
}
