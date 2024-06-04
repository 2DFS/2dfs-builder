package blobstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type blobstore struct {
	path string // path to the blobstore directory
}

type BlobStore interface {
	// GetBlob returns the blob fully qualified file name. Error if it not exists
	GetBlob(digest string) (string, error)
	// Downloads the blob from the store
	DownloadBlob(ctx context.Context, digest string, store string) error
	// UploadBlob generates the empty blob file for a given digest and returns its path.
	UploadBlob(digest string) (string, error)
}

func NewBlobStore(path string) (*blobstore, error) {
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
	return &blobstore{
		path: path,
	}, nil
}

func (b *blobstore) GetBlob(digest string) (io.ReadCloser, error) {
	dest := filepath.Join(b.path, digest)
	openBlob, err := os.Open(dest)
	if err != nil {
		return nil, err
	}
	return openBlob, nil
}

func (b *blobstore) DownloadBlob(ctx context.Context, digest string, store string) error {
	// if path is an URL use Distribution spec to download image index
	// if path is a local file use fsutil.ReadFile
	return nil
}

func (b *blobstore) UploadBlob(digest string) (io.WriteCloser, error) {
	dest := filepath.Join(b.path, digest)
	blobfile, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	defer blobfile.Close()
	return blobfile, nil
}
