package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/2DFS/2dfs-builder/compress"
)

type cachestore struct {
	path string // path to the blobstore directory
	mtx  sync.Mutex
}

type CacheStore interface {
	// Get returns the reader to the cache entry
	Get(digest string) (io.ReadCloser, error)
	//
	GetSize(digest string) (int64, error)
	// Add generates the empty cache entry and returns its writer
	Add(digest string) (io.WriteCloser, error)
	// Del removes the entry from the store
	Del(digest string)
	// Check integrity based on digest
	Check(digest string) bool
	// List all entries in the store
	List() []string
}

func NewCacheStore(path string) (CacheStore, error) {
	storedir, err := os.Open(path)
	defer storedir.Close()
	if err != nil {
		return nil, err
	}
	info, err := storedir.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("blobstore path must be a directory")
	}
	return &cachestore{
		path: path,
		mtx:  sync.Mutex{},
	}, nil
}

func (b *cachestore) Get(digest string) (io.ReadCloser, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	dest := filepath.Join(b.path, digest)
	openBlob, err := os.Open(dest)
	if err != nil {
		return nil, err
	}
	return openBlob, nil
}

func (b *cachestore) GetSize(digest string) (int64, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	dest := filepath.Join(b.path, digest)
	stat, err := os.Stat(dest)
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (b *cachestore) Add(digest string) (io.WriteCloser, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	dest := filepath.Join(b.path, digest)
	blobfile, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	return blobfile, nil
}

func (b *cachestore) Del(digest string) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	dest := filepath.Join(b.path, digest)
	os.Remove(dest)
}

func (b *cachestore) Check(digest string) bool {
	dest := filepath.Join(b.path, digest)
	file, err := os.Open(dest)
	if err != nil {
		return false
	}
	calculatedDigest := compress.CalculateSha256Digest(file)
	if calculatedDigest != digest {
		fmt.Printf("Invalidated cache entry %s\n", digest)
		b.Del(digest)
		return false
	}
	return true
}

func (b *cachestore) List() []string {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	var entries []string
	filepath.Walk(b.path, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			entries = append(entries, info.Name())
		}
		return nil
	})
	return entries
}
