package oci

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/2DFS/2dfs-builder/filesystem"
)

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

func TestNewRemoteCacheKeyCopiesDst(t *testing.T) {
	dst := []string{"./Dockerfile"}

	key := newRemoteCacheKey(
		"file-sha-test",
		dst,
		"compressed-sha-test",
		"diff-id-test",
	)

	dst[0] = "./changed"

	if key.Dst[0] != "./Dockerfile" {
		t.Fatalf("expected newRemoteCacheKey to copy dst slice, got %v", key.Dst)
	}
}

func TestEncodeDecodeRemoteCacheKeyRoundTrip(t *testing.T) {
	original := filesystem.RemoteCacheKey{
		FileSha:       "file-sha-test",
		Dst:           []string{"./Dockerfile", "./requirements.txt"},
		CompressedSha: "compressed-sha-test",
		DiffID:        "diff-id-test",
	}

	reader, err := encodeRemoteCacheKey(original)
	if err != nil {
		t.Fatalf("encodeRemoteCacheKey returned error: %v", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read encoded key: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("expected valid JSON, got %q", string(data))
	}

	decoded, err := decodeRemoteCacheKey(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("decodeRemoteCacheKey returned error: %v", err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("decoded key mismatch\noriginal: %#v\ndecoded:  %#v", original, decoded)
	}
}

func TestDecodeRemoteCacheKeyRejectsInvalidJSON(t *testing.T) {
	if _, err := decodeRemoteCacheKey(strings.NewReader("{invalid-json")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDecodeRemoteCacheKeyRejectsMissingFields(t *testing.T) {
	payload := `{"fileSha":"file-sha-test","dst":["./Dockerfile"]}`

	if _, err := decodeRemoteCacheKey(strings.NewReader(payload)); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestEncodeRemoteCacheKeyRejectsInvalidMetadata(t *testing.T) {
	key := filesystem.RemoteCacheKey{
		FileSha:       "file-sha-test",
		Dst:           []string{"./Dockerfile"},
		CompressedSha: "",
		DiffID:        "diff-id-test",
	}

	if _, err := encodeRemoteCacheKey(key); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateRemoteCacheKeyMatchAcceptsMatchingIdentity(t *testing.T) {
	fileSha := "file-sha-test"
	dst := []string{"./Dockerfile", "./requirements.txt"}

	expectedKeyDigest, err := remoteKeyDigest(fileSha, dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	key := newRemoteCacheKey(
		fileSha,
		dst,
		"compressed-sha-test",
		"diff-id-test",
	)

	if err := validateRemoteCacheKeyMatch(
		key,
		fileSha,
		dst,
		expectedKeyDigest,
	); err != nil {
		t.Fatalf("validateRemoteCacheKeyMatch returned error: %v", err)
	}
}

func TestValidateRemoteCacheKeyMatchRejectsMismatches(t *testing.T) {
	fileSha := "file-sha-test"
	dst := []string{"./Dockerfile"}

	expectedKeyDigest, err := remoteKeyDigest(fileSha, dst)
	if err != nil {
		t.Fatalf("remoteKeyDigest returned error: %v", err)
	}

	tests := []struct {
		name              string
		key               filesystem.RemoteCacheKey
		expectedKeyDigest string
		errorContains     string
	}{
		{
			name: "fileSha mismatch",
			key: newRemoteCacheKey(
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
			key: newRemoteCacheKey(
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
			key: newRemoteCacheKey(
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
			key: newRemoteCacheKey(
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
			err := validateRemoteCacheKeyMatch(
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
