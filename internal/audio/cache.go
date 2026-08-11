package audio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"sync"
	"time"
)

type CacheManager struct {
	cacheDir  string
	maxSizeBytes int64
	mu        sync.RWMutex
}

func NewCacheManager(cacheDir string, maxSizeMB int64) (*CacheManager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	cm := &CacheManager{
		cacheDir:     cacheDir,
		maxSizeBytes: maxSizeMB * 1024 * 1024,
	}

	go cm.enforceLRU()
	return cm, nil
}

func (cm *CacheManager) GetCachedPath(id string) (string, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	filePath := filepath.Join(cm.cacheDir, id+".m4a")
	info, err := os.Stat(filePath)
	if err == nil && info.Size() > 0 {
		// Update access time for LRU
		currentTime := time.Now()
		_ = os.Chtimes(filePath, currentTime, currentTime)
		return filePath, true
	}
	return "", false
}

func (cm *CacheManager) CreateCacheFile(id string) (*os.File, string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	filePath := filepath.Join(cm.cacheDir, id+".m4a")
	file, err := os.Create(filePath)
	if err != nil {
		return nil, "", err
	}
	return file, filePath, nil
}

func (cm *CacheManager) SaveStream(id string, r io.Reader) (string, error) {
	file, filePath, err := cm.CreateCacheFile(id)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		_ = os.Remove(filePath)
		return "", err
	}

	go cm.enforceLRU()
	return filePath, nil
}

func (cm *CacheManager) enforceLRU() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	entries, err := os.ReadDir(cm.cacheDir)
	if err != nil {
		return
	}

	type fileItem struct {
		path    string
		size    int64
		modTime time.Time
	}

	var files []fileItem
	var totalSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		totalSize += info.Size()
		files = append(files, fileItem{
			path:    filepath.Join(cm.cacheDir, entry.Name()),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	if totalSize <= cm.maxSizeBytes {
		return
	}

	// Sort oldest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	for _, file := range files {
		if totalSize <= cm.maxSizeBytes {
			break
		}
		if err := os.Remove(file.path); err == nil {
			totalSize -= file.size
		}
	}
}
