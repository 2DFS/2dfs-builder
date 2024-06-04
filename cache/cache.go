package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type cachestore struct {
	path string // path to the blobstore directory
}

type CacheStore interface {
	// Get returns the reader to the cache entry
	Get(digest string) (io.ReadCloser, error)
	// Add generates the empty cache entry and returns its writer
	Add(digest string) (io.WriteCloser, error)
	// Del removes the entry from the store
	Del(digest string)
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
	}, nil
}

func (b *cachestore) Get(digest string) (io.ReadCloser, error) {
	dest := filepath.Join(b.path, digest)
	openBlob, err := os.Open(dest)
	if err != nil {
		return nil, err
	}
	return openBlob, nil
}

func (b *cachestore) Add(digest string) (io.WriteCloser, error) {
	dest := filepath.Join(b.path, digest)
	blobfile, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	defer blobfile.Close()
	return blobfile, nil
}

func (b *cachestore) Del(digest string) {
	dest := filepath.Join(b.path, digest)
	os.Remove(dest)
}
