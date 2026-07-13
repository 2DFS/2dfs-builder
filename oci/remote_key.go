package oci

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/2DFS/2dfs-builder/filesystem"
)

func remoteKeyDigest(fileSha string, dst []string) (string, error) {
	if err := validateRemoteCacheKeyIdentity(fileSha, dst); err != nil {
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

func newRemoteCacheKey(fileSha string, dst []string, compressedSha string, diffID string) filesystem.RemoteCacheKey {
	return filesystem.RemoteCacheKey{
		FileSha:       fileSha,
		Dst:           append([]string(nil), dst...),
		CompressedSha: compressedSha,
		DiffID:        diffID,
	}
}

func encodeRemoteCacheKey(key filesystem.RemoteCacheKey) (io.Reader, error) {
	if err := validateRemoteCacheKey(key); err != nil {
		return nil, err
	}

	data, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("failed to encode remote cache key: %w", err)
	}

	return bytes.NewReader(data), nil
}

func decodeRemoteCacheKey(reader io.Reader) (filesystem.RemoteCacheKey, error) {
	var key filesystem.RemoteCacheKey

	if err := json.NewDecoder(reader).Decode(&key); err != nil {
		return key, fmt.Errorf("failed to decode remote cache key: %w", err)
	}

	if err := validateRemoteCacheKey(key); err != nil {
		return key, err
	}

	return key, nil

}

func validateRemoteCacheKeyIdentity(fileSha string, dst []string) error {
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

func validateRemoteCacheKey(key filesystem.RemoteCacheKey) error {
	if err := validateRemoteCacheKeyIdentity(key.FileSha, key.Dst); err != nil {
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
