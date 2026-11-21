package startui

import (
	"os"
	"syscall"
	"unsafe"
)

var remoteCacheKernel = syscall.NewLazyDLL("kernel32.dll")

func lockRemoteCacheFile(file *os.File, exclusive bool) error {
	flags := uintptr(1) // LOCKFILE_FAIL_IMMEDIATELY; a shared lock unless exclusive.
	if exclusive {
		flags |= 2
	}
	var overlapped syscall.Overlapped
	ok, _, err := remoteCacheKernel.NewProc("LockFileEx").Call(file.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if ok == 0 {
		if err == syscall.Errno(33) || err == syscall.Errno(32) {
			return errRemoteCacheInUse
		}
		return err
	}
	return nil
}

func remoteCacheFileStorage(path string, info os.FileInfo) (int64, string) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return info.Size(), ""
	}
	var high uint32
	low, _, callErr := remoteCacheKernel.NewProc("GetCompressedFileSizeW").Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&high)))
	if uint32(low) == 0xffffffff && callErr != syscall.Errno(0) {
		return info.Size(), ""
	}
	return int64(uint64(high)<<32 | uint64(uint32(low))), ""
}
