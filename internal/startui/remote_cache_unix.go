//go:build darwin || linux

package startui

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func lockRemoteCacheFile(file *os.File, exclusive bool) error {
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(file.Fd()), mode|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errRemoteCacheInUse
		}
		return err
	}
	return nil
}

func remoteCacheFileStorage(_ string, info os.FileInfo) (int64, string) {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Blocks * 512, fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
	}
	return info.Size(), ""
}
