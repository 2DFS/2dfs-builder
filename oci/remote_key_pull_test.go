package oci

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/2DFS/2dfs-builder/cache"
	"github.com/2DFS/2dfs-builder/filesystem"
)

type fakeRemoteKeyCache struct {
	keyPayload         []byte
	pullKeyErr         error
	returnNilReader    bool
	requestedKeyDigest string
}

func (f *fakeRemoteKeyCache) CheckBlob(compressedSha string) (bool, error) {
	return false, nil
}

func (f *fakeRemoteKeyCache) PushBlob(compressedSha string, reader io.Reader) error {
	return nil
}

func (f *fakeRemoteKeyCache) PullBlob(compressedSha string) (io.ReadCloser, error) {
	return nil, cache.ErrRemoteCacheMiss
}

func (f *fakeRemoteKeyCache) PushKey(keyDigest string, reader io.Reader) error {
	return nil
}

func (f *fakeRemoteKeyCache) PullKey(keyDigest string) (io.ReadCloser, error) {
	f.requestedKeyDigest = keyDigest

	if f.pullKeyErr != nil {
		return nil, f.pullKeyErr
	}

	if f.returnNilReader {
		return nil, nil
	}

	return io.NopCloser(bytes.NewReader(f.keyPayload)), nil
}

func remoteKeyPayload(t *testing.T, key filesystem.RemoteCacheKey) []byte {
	t.Helper()

	payload, err := json.Marshal(key)
	if err != nil {
		t.Fatalf("failed to encode remote key test payload: %v", err)
	}

	return payload
}

func TestPullRemoteCacheKeyReturnsValidatedHit(t *testing.T) {
	fileSha := "file-sha-test"
	dst := []string{"./Dockerfile", "./requirements.txt"}

	expectedKey := newRemoteCacheKey(
		fileSha,
		dst,
		"compressed-sha-test",
		"diff-id-test",
	)

	remoteCache := &fakeRemoteKeyCache{
		keyPayload: remoteKeyPayload(t, expectedKey),
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	key, found, err := container.pullRemoteCacheKey(fileSha, dst)
	if err != nil {
		t.Fatalf("pullRemoteCacheKey returned error: %v", err)
	}

	if !found {
		t.Fatalf("expected remote cache key hit")
	}

	if !reflect.DeepEqual(key, expectedKey) {
		t.Fatalf(
			"unexpected remote key\nexpected: %#v\nactual:   %#v",
			expectedKey,
			key,
		)
	}

	expectedDigest, err := remoteKeyDigest(fileSha, dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	if remoteCache.requestedKeyDigest != expectedDigest {
		t.Fatalf(
			"unexpected requested key digest: expected %q, got %q",
			expectedDigest,
			remoteCache.requestedKeyDigest,
		)
	}
}

func TestPullRemoteCacheKeyReturnsMiss(t *testing.T) {
	remoteCache := &fakeRemoteKeyCache{
		pullKeyErr: cache.ErrRemoteCacheMiss,
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	key, found, err := container.pullRemoteCacheKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
	)
	if err != nil {
		t.Fatalf("expected remote cache miss without error, got: %v", err)
	}

	if found {
		t.Fatalf("expected remote cache miss")
	}

	if !reflect.DeepEqual(key, filesystem.RemoteCacheKey{}) {
		t.Fatalf("expected empty remote cache key, got: %#v", key)
	}
}

func TestPullRemoteCacheKeyReturnsRegistryError(t *testing.T) {
	registryErr := errors.New("registry unavailable")

	remoteCache := &fakeRemoteKeyCache{
		pullKeyErr: registryErr,
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	_, found, err := container.pullRemoteCacheKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
	)

	if err == nil {
		t.Fatalf("expected registry error")
	}

	if found {
		t.Fatalf("expected no cache hit after registry error")
	}

	if !errors.Is(err, registryErr) {
		t.Fatalf("expected wrapped registry error, got: %v", err)
	}
}

func TestPullRemoteCacheKeyRejectsInvalidJSON(t *testing.T) {
	remoteCache := &fakeRemoteKeyCache{
		keyPayload: []byte("{invalid-json"),
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	_, found, err := container.pullRemoteCacheKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
	)

	if err == nil {
		t.Fatalf("expected invalid JSON error")
	}

	if found {
		t.Fatalf("expected no cache hit for invalid JSON")
	}

	if !strings.Contains(err.Error(), "decode remote cache key") {
		t.Fatalf("expected remote key decode error, got: %v", err)
	}
}

func TestPullRemoteCacheKeyRejectsIdentityMismatch(t *testing.T) {
	expectedFileSha := "file-sha-test"
	dst := []string{"./Dockerfile"}

	wrongKey := newRemoteCacheKey(
		"different-file-sha",
		dst,
		"compressed-sha-test",
		"diff-id-test",
	)

	remoteCache := &fakeRemoteKeyCache{
		keyPayload: remoteKeyPayload(t, wrongKey),
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	_, found, err := container.pullRemoteCacheKey(expectedFileSha, dst)
	if err == nil {
		t.Fatalf("expected semantic identity validation error")
	}

	if found {
		t.Fatalf("expected no cache hit for mismatching identity")
	}

	if !strings.Contains(err.Error(), "fileSha mismatch") {
		t.Fatalf("expected fileSha mismatch error, got: %v", err)
	}
}

func TestPullRemoteCacheKeyWithoutRemoteCacheReturnsMiss(t *testing.T) {
	container := &containerImage{}

	key, found, err := container.pullRemoteCacheKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
	)
	if err != nil {
		t.Fatalf("expected no error without remote cache, got: %v", err)
	}

	if found {
		t.Fatalf("expected no cache hit without remote cache")
	}

	if !reflect.DeepEqual(key, filesystem.RemoteCacheKey{}) {
		t.Fatalf("expected empty remote cache key, got: %#v", key)
	}
}

func TestPullRemoteCacheKeyRejectsNilReader(t *testing.T) {
	remoteCache := &fakeRemoteKeyCache{
		returnNilReader: true,
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	_, found, err := container.pullRemoteCacheKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
	)

	if err == nil {
		t.Fatalf("expected nil reader error")
	}

	if found {
		t.Fatalf("expected no cache hit for nil reader")
	}

	if !strings.Contains(err.Error(), "nil reader") {
		t.Fatalf("expected nil reader error, got: %v", err)
	}
}
