package oci

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"context"
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


func TestNewRemoteCacheFromContextDisabledWithoutValues(t *testing.T) {
	ctx := context.Background()

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithoutValues")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: not configured")
	t.Logf("  remote cache repository: not configured")
	t.Logf("  remote cache insecure: not configured")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry and repository are not configured")
	}
}

func TestNewRemoteCacheFromContextDisabledWithEmptyValues(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, false)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithEmptyValues")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: empty string")
	t.Logf("  remote cache repository: empty string")
	t.Logf("  remote cache insecure: false")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry and repository are empty")
	}
}

func TestNewRemoteCacheFromContextDisabledWithOnlyRegistry(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithOnlyRegistry")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: not configured")
	t.Logf("  remote cache insecure: not configured")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when repository is not configured")
	}
}

func TestNewRemoteCacheFromContextDisabledWithOnlyRepository(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithOnlyRepository")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: not configured")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: not configured")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry is not configured")
	}
}

func TestNewRemoteCacheFromContextEnabled(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, true)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextEnabled")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: true")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: non-nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is non-nil: %v", remoteCache != nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache == nil {
		t.Fatal("expected remote cache to be created when registry and repository are configured")
	}
}

func TestNewRemoteCacheFromContextRejectsInvalidInsecureValue(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, "true")

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextRejectsInvalidInsecureValue")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: string(\"true\")")
	t.Logf("EXPECTED:")
	t.Logf("  error: contains \"invalid remote cache insecure value\"")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err == nil {
		t.Fatal("expected error for non-bool remote cache insecure value, got nil")
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when insecure value is invalid")
	}

	if !strings.Contains(err.Error(), "invalid remote cache insecure value") {
		t.Fatalf("expected invalid insecure value error, got: %v", err)
	}
}

func TestNewRemoteCacheFromContextRejectsInvalidRegistryValue(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, 123)
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, false)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextRejectsInvalidRegistryValue")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: int(123)")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: false")
	t.Logf("EXPECTED:")
	t.Logf("  error: contains \"invalid remote cache registry or repository value\"")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err == nil {
		t.Fatal("expected error for non-string remote cache registry value, got nil")
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry value is invalid")
	}

	if !strings.Contains(err.Error(), "invalid remote cache registry or repository value") {
		t.Fatalf("expected invalid registry/repository value error, got: %v", err)
	}
}

func TestNewRemoteCacheFromContextRejectsInvalidRepositoryValue(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, 456)
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, false)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextRejectsInvalidRepositoryValue")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: int(456)")
	t.Logf("  remote cache insecure: false")
	t.Logf("EXPECTED:")
	t.Logf("  error: contains \"invalid remote cache registry or repository value\"")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err == nil {
		t.Fatal("expected error for non-string remote cache repository value, got nil")
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when repository value is invalid")
	}

	if !strings.Contains(err.Error(), "invalid remote cache registry or repository value") {
		t.Fatalf("expected invalid registry/repository value error, got: %v", err)
	}
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

	available, err := container.restoreRemoteBlob(compressedSha)
	if err != nil {
		t.Fatalf("restoreRemoteBlob returned error: %v", err)
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

	available, err := container.restoreRemoteBlob(compressedSha)
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

	available, err := container.restoreRemoteBlob(
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
