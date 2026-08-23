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
