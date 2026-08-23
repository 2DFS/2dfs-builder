package oci

import (
	"testing"

	"github.com/2DFS/2dfs-builder/cache"
)

func newCacheKeyUpsertTestContainer(
	t *testing.T,
) *containerImage {
	t.Helper()

	store, err := cache.NewCacheStore(t.TempDir())
	if err != nil {
		t.Fatalf(
			"failed to create key cache store: %v",
			err,
		)
	}

	return &containerImage{
		keyDigestCache: store,
	}
}

func readStoredCacheKeys(
	t *testing.T,
	container *containerImage,
	fileSha string,
) CacheKeys {
	t.Helper()

	reader, err := container.keyDigestCache.Get(fileSha)
	if err != nil {
		t.Fatalf(
			"failed to read stored cache keys: %v",
			err,
		)
	}
	defer reader.Close()

	keys, err := ParseCacheKey(reader)
	if err != nil {
		t.Fatalf(
			"failed to parse stored cache keys: %v",
			err,
		)
	}

	return keys
}

func TestUpsertCacheKeyReplacesExistingDestination(
	t *testing.T,
) {
	container := newCacheKeyUpsertTestContainer(t)

	fileSha := "file-sha-test"
	dst := []string{"/app/config.json"}

	err := container.upsertLocalCacheKey(
		fileSha,
		FileCacheKey{
			DiffID:        "old-diff-id",
			CompressedSha: "old-compressed-sha",
		},
		dst,
	)
	if err != nil {
		t.Fatalf(
			"first upsertLocalCacheKey call returned error: %v",
			err,
		)
	}

	err = container.upsertLocalCacheKey(
		fileSha,
		FileCacheKey{
			DiffID:        "new-diff-id",
			CompressedSha: "new-compressed-sha",
		},
		dst,
	)
	if err != nil {
		t.Fatalf(
			"second upsertLocalCacheKey call returned error: %v",
			err,
		)
	}

	stored := readStoredCacheKeys(
		t,
		container,
		fileSha,
	)

	if len(stored.Keys) != 1 {
		t.Fatalf(
			"expected one cache key, got %d",
			len(stored.Keys),
		)
	}

	expected := FileCacheKey{
		Destination:   "/app/config.json",
		DiffID:        "new-diff-id",
		CompressedSha: "new-compressed-sha",
	}

	if stored.Keys[0] != expected {
		t.Fatalf(
			"unexpected cache key\nexpected: %#v\nactual:   %#v",
			expected,
			stored.Keys[0],
		)
	}
}

func TestUpsertCacheKeyAppendsNewDestination(
	t *testing.T,
) {
	container := newCacheKeyUpsertTestContainer(t)

	fileSha := "file-sha-test"

	err := container.upsertLocalCacheKey(
		fileSha,
		FileCacheKey{
			DiffID:        "first-diff-id",
			CompressedSha: "first-compressed-sha",
		},
		[]string{"/app/first"},
	)
	if err != nil {
		t.Fatalf(
			"first upsertLocalCacheKey call returned error: %v",
			err,
		)
	}

	err = container.upsertLocalCacheKey(
		fileSha,
		FileCacheKey{
			DiffID:        "second-diff-id",
			CompressedSha: "second-compressed-sha",
		},
		[]string{"/app/second"},
	)
	if err != nil {
		t.Fatalf(
			"second upsertLocalCacheKey call returned error: %v",
			err,
		)
	}

	stored := readStoredCacheKeys(
		t,
		container,
		fileSha,
	)

	if len(stored.Keys) != 2 {
		t.Fatalf(
			"expected two cache keys, got %d",
			len(stored.Keys),
		)
	}

	if stored.Keys[0].Destination != "/app/first" {
		t.Fatalf(
			"unexpected first destination: %q",
			stored.Keys[0].Destination,
		)
	}

	if stored.Keys[1].Destination != "/app/second" {
		t.Fatalf(
			"unexpected second destination: %q",
			stored.Keys[1].Destination,
		)
	}
}

func TestUpsertFileCacheKeyRemovesDuplicateDestinations(
	t *testing.T,
) {
	cacheKeys := CacheKeys{
		Keys: []FileCacheKey{
			{
				Destination:   "/app/config.json",
				DiffID:        "old-diff-id-1",
				CompressedSha: "old-compressed-sha-1",
			},
			{
				Destination:   "/app/other.json",
				DiffID:        "other-diff-id",
				CompressedSha: "other-compressed-sha",
			},
			{
				Destination:   "/app/config.json",
				DiffID:        "old-diff-id-2",
				CompressedSha: "old-compressed-sha-2",
			},
		},
	}

	newKey := FileCacheKey{
		Destination:   "/app/config.json",
		DiffID:        "new-diff-id",
		CompressedSha: "new-compressed-sha",
	}

	updated := upsertFileCacheKey(cacheKeys, newKey)

	if len(updated.Keys) != 2 {
		t.Fatalf(
			"expected two unique destinations, got %d",
			len(updated.Keys),
		)
	}

	if updated.Keys[0] != newKey {
		t.Fatalf(
			"unexpected replaced key\nexpected: %#v\nactual:   %#v",
			newKey,
			updated.Keys[0],
		)
	}

	expectedOther := FileCacheKey{
		Destination:   "/app/other.json",
		DiffID:        "other-diff-id",
		CompressedSha: "other-compressed-sha",
	}

	if updated.Keys[1] != expectedOther {
		t.Fatalf(
			"unrelated key was modified\nexpected: %#v\nactual:   %#v",
			expectedOther,
			updated.Keys[1],
		)
	}
}
