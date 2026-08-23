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

	pushKeyErr      error
	pushKeyCalls    int
	pushedKeyDigest string
	pushedKeyData   []byte
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
	f.pushKeyCalls++
	f.pushedKeyDigest = keyDigest

	if f.pushKeyErr != nil {
		return f.pushKeyErr
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	f.pushedKeyData = append([]byte(nil), data...)

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

func remoteKeyPayload(t *testing.T, key filesystem.RemoteKey) []byte {
	t.Helper()

	payload, err := json.Marshal(key)
	if err != nil {
		t.Fatalf("failed to encode remote key test payload: %v", err)
	}

	return payload
}

func TestLookupRemoteKeyReturnsValidatedHit(t *testing.T) {
	fileSha := "file-sha-test"
	dst := []string{"./Dockerfile", "./requirements.txt"}

	expectedKey := newRemoteKey(
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

	key, found, err := container.lookupRemoteKey(fileSha, dst)
	if err != nil {
		t.Fatalf("lookupRemoteKey returned error: %v", err)
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

func TestLookupRemoteKeyReturnsMiss(t *testing.T) {
	remoteCache := &fakeRemoteKeyCache{
		pullKeyErr: cache.ErrRemoteCacheMiss,
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	key, found, err := container.lookupRemoteKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
	)
	if err != nil {
		t.Fatalf("expected remote cache miss without error, got: %v", err)
	}

	if found {
		t.Fatalf("expected remote cache miss")
	}

	if !reflect.DeepEqual(key, filesystem.RemoteKey{}) {
		t.Fatalf("expected empty remote cache key, got: %#v", key)
	}
}

func TestLookupRemoteKeyReturnsRegistryError(t *testing.T) {
	registryErr := errors.New("registry unavailable")

	remoteCache := &fakeRemoteKeyCache{
		pullKeyErr: registryErr,
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	_, found, err := container.lookupRemoteKey(
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

func TestLookupRemoteKeyRejectsInvalidJSON(t *testing.T) {
	remoteCache := &fakeRemoteKeyCache{
		keyPayload: []byte("{invalid-json"),
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	_, found, err := container.lookupRemoteKey(
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

func TestLookupRemoteKeyRejectsIdentityMismatch(t *testing.T) {
	expectedFileSha := "file-sha-test"
	dst := []string{"./Dockerfile"}

	wrongKey := newRemoteKey(
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

	_, found, err := container.lookupRemoteKey(expectedFileSha, dst)
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

func TestLookupRemoteKeyWithoutRemoteCacheReturnsMiss(t *testing.T) {
	container := &containerImage{}

	key, found, err := container.lookupRemoteKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
	)
	if err != nil {
		t.Fatalf("expected no error without remote cache, got: %v", err)
	}

	if found {
		t.Fatalf("expected no cache hit without remote cache")
	}

	if !reflect.DeepEqual(key, filesystem.RemoteKey{}) {
		t.Fatalf("expected empty remote cache key, got: %#v", key)
	}
}

func TestLookupRemoteKeyRejectsNilReader(t *testing.T) {
	remoteCache := &fakeRemoteKeyCache{
		returnNilReader: true,
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	_, found, err := container.lookupRemoteKey(
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

func TestPublishRemoteKeyPublishesExpectedMetadata(t *testing.T) {
	fileSha := "file-sha-test"
	dst := []string{"./Dockerfile", "./requirements.txt"}
	compressedSha := "compressed-sha-test"
	diffID := "diff-id-test"

	remoteCache := &fakeRemoteKeyCache{}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	err := container.publishRemoteKey(
		fileSha,
		dst,
		compressedSha,
		diffID,
	)
	if err != nil {
		t.Fatalf("publishRemoteKey returned error: %v", err)
	}

	if remoteCache.pushKeyCalls != 1 {
		t.Fatalf(
			"expected PushKey to be called once, got %d",
			remoteCache.pushKeyCalls,
		)
	}

	expectedDigest, err := remoteKeyDigest(fileSha, dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	if remoteCache.pushedKeyDigest != expectedDigest {
		t.Fatalf(
			"unexpected pushed key digest: expected %q, got %q",
			expectedDigest,
			remoteCache.pushedKeyDigest,
		)
	}

	actualKey, err := decodeRemoteKey(
		bytes.NewReader(remoteCache.pushedKeyData),
	)
	if err != nil {
		t.Fatalf("failed to decode pushed remote key: %v", err)
	}

	expectedKey := newRemoteKey(
		fileSha,
		dst,
		compressedSha,
		diffID,
	)

	if !reflect.DeepEqual(actualKey, expectedKey) {
		t.Fatalf(
			"unexpected pushed remote key\nexpected: %#v\nactual:   %#v",
			expectedKey,
			actualKey,
		)
	}
}

func TestPublishRemoteKeyReturnsPushError(t *testing.T) {
	pushErr := errors.New("remote key push failed")

	remoteCache := &fakeRemoteKeyCache{
		pushKeyErr: pushErr,
	}

	container := &containerImage{
		remoteCache: remoteCache,
	}

	err := container.publishRemoteKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
		"compressed-sha-test",
		"diff-id-test",
	)

	if err == nil {
		t.Fatalf("expected publishRemoteKey to return an error")
	}

	if !errors.Is(err, pushErr) {
		t.Fatalf(
			"expected wrapped PushKey error, got: %v",
			err,
		)
	}

	if remoteCache.pushKeyCalls != 1 {
		t.Fatalf(
			"expected PushKey to be called once, got %d",
			remoteCache.pushKeyCalls,
		)
	}
}

func TestPublishRemoteKeyWithoutRemoteCacheDoesNothing(t *testing.T) {
	container := &containerImage{}

	err := container.publishRemoteKey(
		"file-sha-test",
		[]string{"./Dockerfile"},
		"compressed-sha-test",
		"diff-id-test",
	)

	if err != nil {
		t.Fatalf(
			"expected no error without remote cache, got: %v",
			err,
		)
	}
}
