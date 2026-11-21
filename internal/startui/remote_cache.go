package startui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

var errRemoteCacheInUse = errors.New("Local image cache is in use by another build, upload or cleanup. Wait for it to finish and retry.")

type remoteCacheStats struct {
	Bytes        int64 `json:"bytes"`
	Entries      int   `json:"entries"`
	Files        int   `json:"files"`
	InUse        bool  `json:"inUse"`
	Clearing     bool  `json:"clearing"`
	Measured     bool  `json:"measured"`
	RemovedBytes int64 `json:"removedBytes,omitempty"`
}

// The lock lives outside remote-images so cleanup never removes its inode.
// OS locks are released even when a launcher crashes. Shared leases cover both
// image generation and use of its files during transfer; cleanup is exclusive.
func lockRemoteCache(exclusive bool) (func(), error) {
	if err := os.MkdirAll(settingsDir(), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(settingsDir(), "remote-image-cache.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockRemoteCacheFile(file, exclusive); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = file.Close() }, nil
}

func openRemoteCache() (*os.Root, error) {
	path := filepath.Join(settingsDir(), "remote-images")
	original, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !original.IsDir() || original.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Local image cache is not a regular directory; it has not been changed.")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	current, err := root.Stat(".")
	if err != nil || !os.SameFile(original, current) {
		root.Close()
		return nil, errors.New("Local image cache directory changed while opening it; retry.")
	}
	return root, nil
}

func remoteCacheDirectories(root *os.Root) ([]string, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		// Unknown files and symlinks at the cache root are never counted or deleted.
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && remoteImageIDPattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func measureRemoteCache(ctx context.Context) (remoteCacheStats, error) {
	result := remoteCacheStats{Measured: true}
	root, err := openRemoteCache()
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer root.Close()
	names, err := remoteCacheDirectories(root)
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for _, name := range names {
		result.Entries++
		err = fs.WalkDir(root.FS(), name, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if os.IsNotExist(walkErr) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			info, err := entry.Info()
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			result.Files++
			size, identity := remoteCacheFileStorage(filepath.Join(root.Name(), filepath.FromSlash(path)), info)
			if identity != "" {
				if seen[identity] {
					return nil
				}
				seen[identity] = true
			}
			result.Bytes += size
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func localRemoteCacheUsage(ctx context.Context) (remoteCacheStats, error) {
	inUse := false
	unlock, err := lockRemoteCache(true)
	if errors.Is(err, errRemoteCacheInUse) {
		inUse = true
	} else if err != nil {
		return remoteCacheStats{}, err
	} else {
		unlock()
	}
	unlock, err = lockRemoteCache(false)
	if errors.Is(err, errRemoteCacheInUse) {
		return remoteCacheStats{InUse: true}, nil
	}
	if err != nil {
		return remoteCacheStats{}, err
	}
	defer unlock()
	result, err := measureRemoteCache(ctx)
	result.InUse = inUse
	return result, err
}

func clearLocalRemoteCache(ctx context.Context) (remoteCacheStats, error) {
	unlock, err := lockRemoteCache(true)
	if err != nil {
		return remoteCacheStats{}, err
	}
	defer unlock()
	before, err := measureRemoteCache(ctx)
	if err != nil {
		return before, err
	}
	root, err := openRemoteCache()
	if os.IsNotExist(err) {
		return before, nil
	}
	if err != nil {
		return before, err
	}
	defer root.Close()
	names, err := remoteCacheDirectories(root)
	if err != nil {
		return before, err
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return remoteCacheStats{}, err
		}
		// Root anchors deletion to the opened cache directory. RemoveAll unlinks
		// nested symlinks without following them into projects or other storage.
		if err := root.RemoveAll(name); err != nil {
			return remoteCacheStats{}, fmt.Errorf("Clear local image cache: %w", err)
		}
	}
	after, err := measureRemoteCache(ctx)
	after.RemovedBytes = before.Bytes - after.Bytes
	if after.RemovedBytes < 0 {
		after.RemovedBytes = 0
	}
	return after, err
}

func (p *remoteController) serveLocalRemoteCache(w http.ResponseWriter, r *http.Request, clear bool, local *logState, prerequisites *prerequisiteSetup) {
	w.Header().Set("Cache-Control", "no-store")
	if !clear {
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", 405)
			return
		}
		result, err := localRemoteCacheUsage(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		p.Lock()
		result.Clearing = p.cacheClearing
		result.InUse = result.InUse || p.state.Running
		p.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	local.Lock()
	prerequisites.Lock()
	installing := prerequisites.installing
	prerequisites.Unlock()
	p.Lock()
	if local.Running || installing || p.state.Running || p.stopping || p.cacheClearing {
		p.Unlock()
		local.Unlock()
		http.Error(w, "Wait for startup, installation or cache cleanup to finish.", 409)
		return
	}
	p.cacheClearing = true
	p.Unlock()
	local.Unlock()
	defer func() { p.Lock(); p.cacheClearing = false; p.Unlock() }()
	result, err := clearLocalRemoteCache(r.Context())
	if err != nil {
		code := 500
		if errors.Is(err, errRemoteCacheInUse) {
			code = 409
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
