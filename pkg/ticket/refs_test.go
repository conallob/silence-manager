package ticket

import "testing"

func TestSharedExtractSilenceRef(t *testing.T) {
	tests := []struct {
		name, desc, prefix, want string
	}{
		{"first line", "silence-manager: abc\n\nbody", "silence-manager", "abc"},
		{"later line", "Hello\nsilence-manager: abc\nmore", "silence-manager", "abc"},
		{"indented and padded", "  silence-manager:  abc  ", "silence-manager", "abc"},
		{"custom prefix", "my.prefix+x: id-1", "my.prefix+x", "id-1"},
		{"prefix is not a regex", "myXprefix+x: id-1", "my.prefix+x", ""},
		{"other prefix ignored", "other: abc", "silence-manager", ""},
		{"mid-line mention ignored", "see silence-manager: abc", "silence-manager", ""},
		{"empty", "", "silence-manager", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSilenceRef(tt.desc, tt.prefix); got != tt.want {
				t.Errorf("extractSilenceRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSharedWithSilenceRef(t *testing.T) {
	if got := withSilenceRef("body", "", "p"); got != "body" {
		t.Errorf("Expected description unchanged without a ref, got %q", got)
	}
	got := withSilenceRef("body", "s1", "p")
	if got != "p: s1\n\nbody" {
		t.Errorf("Unexpected annotated description: %q", got)
	}
	if extractSilenceRef(got, "p") != "s1" {
		t.Error("Annotation should round-trip")
	}
}

func TestSharedWithSilenceRef_ReplacesExistingAnnotation(t *testing.T) {
	existing := withSilenceRef("body", "old", "p")
	updated := withSilenceRef(existing, "new", "p")
	if updated != "p: new\n\nbody" {
		t.Errorf("Expected the old annotation to be replaced, got %q", updated)
	}
	if again := withSilenceRef(updated, "new", "p"); again != updated {
		t.Errorf("Annotating twice should be idempotent, got %q", again)
	}
}
