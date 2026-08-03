package oci

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/2DFS/2dfs-builder/cache"
)

type fakeRemoteCache struct {
	blob   []byte
	reader io.ReadCloser
	err    error
}

func (f fakeRemoteCache) CheckBlob(compressedSha string) (bool, error) {
	return false, nil
}

func (f fakeRemoteCache) PushBlob(compressedSha string, r io.Reader) error {
	return nil
}

func (f fakeRemoteCache) PullBlob(compressedSha string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}

	if f.reader != nil {
		return f.reader, nil
	}

	return io.NopCloser(bytes.NewReader(f.blob)), nil
}

func (f fakeRemoteCache) PushKey(keyDigest string, reader io.Reader) error {
	return nil
}

func (f fakeRemoteCache) PullKey(keyDigest string) (io.ReadCloser, error) {
	return nil, cache.ErrRemoteCacheMiss
}

type failingReadCloser struct {
	err error
}

func (r *failingReadCloser) Read(p []byte) (int, error) {
	return 0, r.err
}

func (r *failingReadCloser) Close() error {
	return nil
}

func testSHA256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:])
}

func TestPullRemoteBlobToLocalCacheRestoresMatchingDigest(t *testing.T) {
	blob := []byte("valid remote blob content")
	compressedSha := testSHA256Hex(blob)

	blobCacheDir := t.TempDir()
	blobCache, err := cache.NewCacheStore(blobCacheDir)
	if err != nil {
		t.Fatalf("NewCacheStore returned error: %v", err)
	}

	container := &containerImage{
		blobCache: blobCache,
		remoteCache: fakeRemoteCache{
			blob: blob,
		},
	}

	available, err := container.pullRemoteBlobToLocalCache(compressedSha)
	if err != nil {
		t.Fatalf("pullRemoteBlobToLocalCache returned error: %v", err)
	}

	if !available {
		t.Fatalf("expected blob to be restored from remote cache")
	}

	restoredBlob, err := os.ReadFile(filepath.Join(blobCacheDir, compressedSha))
	if err != nil {
		t.Fatalf("failed to read restored blob: %v", err)
	}

	if !bytes.Equal(restoredBlob, blob) {
		t.Fatalf("restored blob mismatch")
	}
}

func TestPullRemoteBlobToLocalCacheRejectsDigestMismatch(t *testing.T) {
	expectedBlob := []byte("expected blob content")
	pulledBlob := []byte("different remote blob content")
	compressedSha := testSHA256Hex(expectedBlob)

	blobCacheDir := t.TempDir()
	blobCache, err := cache.NewCacheStore(blobCacheDir)
	if err != nil {
		t.Fatalf("NewCacheStore returned error: %v", err)
	}

	container := &containerImage{
		blobCache: blobCache,
		remoteCache: fakeRemoteCache{
			blob: pulledBlob,
		},
	}

	available, err := container.pullRemoteBlobToLocalCache(compressedSha)
	if err == nil {
		t.Fatalf("expected digest mismatch error")
	}

	if available {
		t.Fatalf("expected blob restore to be unavailable after digest mismatch")
	}

	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch error, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(blobCacheDir, compressedSha)); !os.IsNotExist(statErr) {
		t.Fatalf("expected mismatched local blob to be deleted")
	}
}

func TestPullRemoteBlobToLocalCacheReturnsStreamError(t *testing.T) {
	remoteReadErr := errors.New("remote stream interrupted")
	compressedSha := testSHA256Hex([]byte("expected blob content"))

	blobCacheDir := t.TempDir()
	blobCache, err := cache.NewCacheStore(blobCacheDir)
	if err != nil {
		t.Fatalf("NewCacheStore returned error: %v", err)
	}

	container := &containerImage{
		blobCache: blobCache,
		remoteCache: fakeRemoteCache{
			reader: &failingReadCloser{
				err: remoteReadErr,
			},
		},
	}

	available, err := container.pullRemoteBlobToLocalCache(
		compressedSha,
	)

	if err == nil {
		t.Fatal("expected remote stream error")
	}

	if available {
		t.Fatal("expected blob to remain unavailable")
	}

	if !errors.Is(err, remoteReadErr) {
		t.Fatalf(
			"expected wrapped remote stream error, got: %v",
			err,
		)
	}

	blobPath := filepath.Join(blobCacheDir, compressedSha)
	if _, statErr := os.Stat(blobPath); !os.IsNotExist(statErr) {
		t.Fatal("expected incomplete local blob entry to be deleted")
	}
}
