package oci

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/2DFS/2dfs-builder/filesystem"
)

func remoteKeyDigest(fileSha string, dst []string) (string, error) {
	if err := validateRemoteKeyIdentity(fileSha, dst); err != nil {
		return "", err
	}

	input := struct {
		FileSha string   `json:"fileSha"`
		Dst     []string `json:"dst"`
	}{
		FileSha: fileSha,
		Dst:     append([]string(nil), dst...),
	}

	data, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to marshal remote key digest input: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func newRemoteKey(fileSha string, dst []string, compressedSha string, diffID string) filesystem.RemoteKey {
	return filesystem.RemoteKey{
		FileSha:       fileSha,
		Dst:           append([]string(nil), dst...),
		CompressedSha: compressedSha,
		DiffID:        diffID,
	}
}

func encodeRemoteKey(key filesystem.RemoteKey) (io.Reader, error) {
	if err := validateRemoteKey(key); err != nil {
		return nil, err
	}

	data, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("failed to encode remote cache key: %w", err)
	}

	return bytes.NewReader(data), nil
}

func decodeRemoteKey(reader io.Reader) (filesystem.RemoteKey, error) {
	var key filesystem.RemoteKey

	if err := json.NewDecoder(reader).Decode(&key); err != nil {
		return key, fmt.Errorf("failed to decode remote cache key: %w", err)
	}

	if err := validateRemoteKey(key); err != nil {
		return key, err
	}

	return key, nil

}

func validateRemoteKeyIdentity(fileSha string, dst []string) error {
	if fileSha == "" {
		return fmt.Errorf("remote cache key fileSha is empty")
	}

	if len(dst) == 0 {
		return fmt.Errorf("remote cache key dst is empty")
	}

	for _, d := range dst {
		if d == "" {
			return fmt.Errorf("remote cache key contains empty dst entry")
		}
	}

	return nil
}

func validateRemoteKeyMatch(key filesystem.RemoteKey, expectedFileSha string, expectedDst []string, expectedKeyDigest string) error {
	if err := validateRemoteKey(key); err != nil {
		return fmt.Errorf("pulled remote cache key is invalid: %w", err)
	}

	if key.FileSha != expectedFileSha {
		return fmt.Errorf("remote cache key fileSha mismatch: expected %q, got %q", expectedFileSha, key.FileSha)
	}

	if !slices.Equal(key.Dst, expectedDst) {
		return fmt.Errorf("remote cache key dst mismatch: expected %v, got %v", expectedDst, key.Dst)
	}

	actualKeyDigest, err := remoteKeyDigest(key.FileSha, key.Dst)
	if err != nil {
		return fmt.Errorf("failed to calculate pulled remote cache key digest: %w", err)
	}

	if actualKeyDigest != expectedKeyDigest {
		return fmt.Errorf("remote cache key digest mismatch: expected %q, got %q", expectedKeyDigest, actualKeyDigest)
	}

	return nil
}

func validateRemoteKey(key filesystem.RemoteKey) error {
	if err := validateRemoteKeyIdentity(key.FileSha, key.Dst); err != nil {
		return err
	}

	if key.CompressedSha == "" {
		return fmt.Errorf("remote cache key compressedSha is empty")
	}

	if key.DiffID == "" {
		return fmt.Errorf("remote cache key diffID is empty")
	}

	return nil
}
