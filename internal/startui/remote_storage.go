package startui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const remoteBuildMinimumFree = uint64(20 << 30)
const remoteBuildReserveFree = uint64(2 << 30)

var remoteStorageAvailable = freeDiskBytes

func checkRemoteBuildSpace(path string, minimum uint64) error {
	available, err := remoteStorageAvailable(path)
	if err != nil {
		return fmt.Errorf("Cannot check image-build storage at %s: %w", path, err)
	}
	if available < minimum {
		return fmt.Errorf("Not enough free disk space for a remote image build: %.1f GiB available at %s; at least %.0f GiB required. Free space on this drive, or select a cached image on the HPC. No project files or existing images were deleted", float64(available)/(1<<30), path, float64(minimum)/(1<<30))
	}
	return nil
}

// Stop only this temporary builder if other applications consume the host reserve.
func watchRemoteBuildSpace(ctx context.Context, cancel context.CancelFunc, path string, report chan<- error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := checkRemoteBuildSpace(path, remoteBuildReserveFree); err != nil {
				report <- err
				cancel()
				return
			}
		}
	}
}

type remoteBuildOutput struct {
	sync.Mutex
	target io.Writer
	tail   string
}

func (w *remoteBuildOutput) Write(data []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	w.tail += string(data)
	if len(w.tail) > 16384 {
		w.tail = w.tail[len(w.tail)-16384:]
	}
	return w.target.Write(data)
}
func (w *remoteBuildOutput) failure(err error) error {
	w.Lock()
	defer w.Unlock()
	output := strings.ToLower(w.tail)
	for _, marker := range []string{"no space left on device", "input/output error", "disk quota exceeded", "not enough free disk space"} {
		if strings.Contains(output, marker) {
			return fmt.Errorf("Docker image-build storage failed (%s). Check free space on the host and inside the temporary Docker Sandbox, and check storage health if space is sufficient. The temporary builder is removed; free space before retrying: %w", marker, err)
		}
	}
	return fmt.Errorf("Build remote image inside Docker Sandbox (check architecture/emulation and Dockerfile dependencies): %w", err)
}
