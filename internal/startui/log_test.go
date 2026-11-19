package startui

import "testing"

func TestActivityStripsTerminalFormattingAcrossChunks(t *testing.T) {
	s := &logState{}
	s.Write([]byte("\x1b[38;2;12"))
	s.Write([]byte("2;162;247mDocker ready\x1b[m https://example.org\x1b["))
	s.Write([]byte("0m\n"))
	if s.Text != "Docker ready https://example.org\n" {
		t.Fatalf("activity contains terminal codes: %q", s.Text)
	}
}
