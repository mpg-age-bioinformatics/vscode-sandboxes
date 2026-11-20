package startui

import (
	"fmt"
	"io"
	"time"
)

// Counts the existing SSH stdin stream. No additional reads, requests or remote
// polling are used. Bytes sent are not declared checksum-verified until SSH exits.
type remoteTransferReader struct {
	source        io.Reader
	log           io.Writer
	name          string
	total, sent   int64
	started, last time.Time
	now           func() time.Time
}

func (r *remoteTransferReader) Read(buffer []byte) (int, error) {
	n, err := r.source.Read(buffer)
	r.sent += int64(n)
	now := r.now()
	if n > 0 && (now.Sub(r.last) >= 2*time.Second || r.sent == r.total) {
		r.last = now
		elapsed := now.Sub(r.started).Seconds()
		rate := float64(r.sent) / elapsed
		percent := float64(r.sent) * 100 / float64(r.total)
		if elapsed > 0 && rate > 0 {
			eta := time.Duration(float64(r.total-r.sent) / rate * float64(time.Second)).Round(time.Second)
			fmt.Fprintf(r.log, "Upload %s: %.1f%% (%.1f / %.1f MiB), %.1f MiB/s, ~%s remaining\n", r.name, percent, float64(r.sent)/(1<<20), float64(r.total)/(1<<20), rate/(1<<20), eta)
		}
		if r.sent == r.total {
			fmt.Fprintln(r.log, "All archive bytes streamed; waiting for the remote SHA-256 check to finish…")
		}
	}
	return n, err
}
