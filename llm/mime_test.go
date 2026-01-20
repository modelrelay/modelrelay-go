package llm

import "testing"

func TestFileKindFromMimeType_Text(t *testing.T) {
	if got := FileKindFromMimeType(MimeType("text/plain")); got != FileKindText {
		t.Fatalf("expected %q, got %q", FileKindText, got)
	}
}
