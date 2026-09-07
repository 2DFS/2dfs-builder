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

func TestRemoteKeyDigestIsDeterministic(t *testing.T) {
	dst := []string{"./Dockerfile", "./requirements.txt"}

	first, err := remoteKeyDigest("file-sha-test", dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	second, err := remoteKeyDigest("file-sha-test", dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	if first != second {
		t.Fatalf("expected deterministic digest, got %q and %q", first, second)
	}

	if len(first) != 64 {
		t.Fatalf("expected sha256 hex digest length 64, got %d", len(first))
	}
}

func TestRemoteKeyDigestChangesWhenInputChanges(t *testing.T) {
	base, err := remoteKeyDigest("file-sha-test", []string{"./Dockerfile"})
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	changedFileSha, err := remoteKeyDigest("different-file-sha", []string{"./Dockerfile"})
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	changedDst, err := remoteKeyDigest("file-sha-test", []string{"./requirements.txt"})
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	if base == changedFileSha {
		t.Fatalf("expected digest to change when fileSha changes")
	}

	if base == changedDst {
		t.Fatalf("expected digest to change when dst changes")
	}
}

func TestRemoteKeyDigestRejectsInvalidIdentity(t *testing.T) {
	tests := []struct {
		name    string
		fileSha string
		dst     []string
	}{
		{
			name:    "empty fileSha",
			fileSha: "",
			dst:     []string{"./Dockerfile"},
		},
		{
			name:    "empty dst list",
			fileSha: "file-sha-test",
			dst:     nil,
		},
		{
			name:    "empty dst entry",
			fileSha: "file-sha-test",
			dst:     []string{"./Dockerfile", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := remoteKeyDigest(tt.fileSha, tt.dst); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestNewRemoteKeyCopiesDst(t *testing.T) {
	dst := []string{"./Dockerfile"}

	key := newRemoteKey(
		"file-sha-test",
		dst,
		"compressed-sha-test",
		"diff-id-test",
	)

	dst[0] = "./changed"

	if key.Dst[0] != "./Dockerfile" {
		t.Fatalf("expected newRemoteKey to copy dst slice, got %v", key.Dst)
	}
}

func TestEncodeDecodeRemoteKeyRoundTrip(t *testing.T) {
	original := filesystem.RemoteKey{
		FileSha:       "file-sha-test",
		Dst:           []string{"./Dockerfile", "./requirements.txt"},
		CompressedSha: "compressed-sha-test",
		DiffID:        "diff-id-test",
	}

	reader, err := encodeRemoteKey(original)
	if err != nil {
		t.Fatalf("encodeRemoteKey returned error: %v", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read encoded key: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("expected valid JSON, got %q", string(data))
	}

	decoded, err := decodeRemoteKey(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("decodeRemoteKey returned error: %v", err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("decoded key mismatch\noriginal: %#v\ndecoded:  %#v", original, decoded)
	}
}

func TestDecodeRemoteKeyRejectsInvalidJSON(t *testing.T) {
	if _, err := decodeRemoteKey(strings.NewReader("{invalid-json")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDecodeRemoteKeyRejectsMissingFields(t *testing.T) {
	payload := `{"fileSha":"file-sha-test","dst":["./Dockerfile"]}`

	if _, err := decodeRemoteKey(strings.NewReader(payload)); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestEncodeRemoteKeyRejectsInvalidMetadata(t *testing.T) {
	key := filesystem.RemoteKey{
		FileSha:       "file-sha-test",
		Dst:           []string{"./Dockerfile"},
		CompressedSha: "",
		DiffID:        "diff-id-test",
	}

	if _, err := encodeRemoteKey(key); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateRemoteKeyMatchAcceptsMatchingIdentity(t *testing.T) {
	fileSha := "file-sha-test"
	dst := []string{"./Dockerfile", "./requirements.txt"}

	expectedKeyDigest, err := remoteKeyDigest(fileSha, dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	key := newRemoteKey(
		fileSha,
		dst,
		"compressed-sha-test",
		"diff-id-test",
	)

	if err := validateRemoteKeyMatch(
		key,
		fileSha,
		dst,
		expectedKeyDigest,
	); err != nil {
		t.Fatalf("validateRemoteKeyMatch returned error: %v", err)
	}
}

func TestValidateRemoteKeyMatchRejectsMismatches(t *testing.T) {
	fileSha := "file-sha-test"
	dst := []string{"./Dockerfile"}

	expectedKeyDigest, err := remoteKeyDigest(fileSha, dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	tests := []struct {
		name              string
		key               filesystem.RemoteKey
		expectedKeyDigest string
		errorContains     string
	}{
		{
			name: "fileSha mismatch",
			key: newRemoteKey(
				"different-file-sha",
				dst,
				"compressed-sha-test",
				"diff-id-test",
			),
			expectedKeyDigest: expectedKeyDigest,
			errorContains:     "fileSha mismatch",
		},
		{
			name: "dst mismatch",
			key: newRemoteKey(
				fileSha,
				[]string{"./requirements.txt"},
				"compressed-sha-test",
				"diff-id-test",
			),
			expectedKeyDigest: expectedKeyDigest,
			errorContains:     "dst mismatch",
		},
		{
			name: "key digest mismatch",
			key: newRemoteKey(
				fileSha,
				dst,
				"compressed-sha-test",
				"diff-id-test",
			),
			expectedKeyDigest: strings.Repeat("0", 64),
			errorContains:     "digest mismatch",
		},
		{
			name: "invalid metadata",
			key: newRemoteKey(
				fileSha,
				dst,
				"",
				"diff-id-test",
			),
			expectedKeyDigest: expectedKeyDigest,
			errorContains:     "pulled remote cache key is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRemoteKeyMatch(
				tt.key,
				fileSha,
				dst,
				tt.expectedKeyDigest,
			)
			if err == nil {
				t.Fatalf("expected error")
			}

			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Fatalf(
					"expected error containing %q, got %q",
					tt.errorContains,
					err,
				)
			}
		})
	}
}
