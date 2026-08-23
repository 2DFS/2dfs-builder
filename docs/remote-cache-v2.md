# remote-cache-v2

## Scope

This document describes the design and current implementation status of the OCI registry-backed remote cache for the 2DFS builder.

The broader goal is to make 2DFS cache data reusable across different machines and build environments. The existing local cache is tied to one machine. Using an OCI-compatible registry as a remote cache enables cache sharing in distributed or stateless environments such as CI/CD runners, edge devices, and ephemeral workers.

## Design Summary

Every cache entry represents one 2DFS allotment. The remote cache is stored in an OCI registry repository separate from the final image repository, similar in purpose to the local `.blobcache` and key-cache stores.

The design uses two remote cache entities:

```text
key-sha256-<keyDigest>
blob-sha256-<compressedSha>
```

The two entities have different identities and responsibilities:

- **Blob entity:** stores the compressed allotment blob and is addressed by `compressedSha`.
- **Key entity:** stores metadata that maps the semantic allotment identity to the blob and is addressed by `keyDigest`.

The remote key digest is derived deterministically from:

```text
fileSha + dst
    ↓
SHA-256(JSON({fileSha, dst}))
    ↓
keyDigest
```

The corresponding remote key payload contains:

```json
{
  "fileSha": "...",
  "dst": ["..."],
  "compressedSha": "...",
  "diffID": "..."
}
```

This separates **lookup identity** (`fileSha + dst`) from **blob identity** (`compressedSha`). A builder can therefore resolve which blob belongs to an allotment without rebuilding it first.

### OCI representation

Both remote cache entities are represented as independently addressable single-layer OCI images.

For blob entries, the compressed allotment blob is the single OCI layer. For key entries, the JSON metadata is stored as the single layer.

This keeps the remote cache compatible with standard OCI registries without requiring registry-side changes.

### Deterministic tag scheme

Blob tags are derived directly from the compressed blob digest:

```text
blob-sha256-<compressedSha>
```

Key tags are derived from the deterministic semantic key digest:

```text
key-sha256-<keyDigest>
```

The builder can therefore perform direct tag lookups without listing or scanning the cache repository.

### Separate cache repository

The remote cache can be stored in a repository separate from final target images. This keeps cache artifacts isolated from normal image tags and prevents cache entries from cluttering the target image repository.

Example layout:

```text
localhost:5000/2dfs/cache:blob-sha256-...
localhost:5000/2dfs/cache:key-sha256-...
localhost:5000/2dfs/my-image:v1
```

### No central mutable cache index

A central mutable JSON index or shared cache manifest is intentionally avoided. Such an index would require read-modify-write coordination between distributed builders and could introduce race conditions when multiple workers publish cache entries concurrently.

Each remote key and blob is independently addressable through a deterministic OCI tag. Updating or publishing one entry therefore does not require rewriting a shared index.

### Blob validation

A remote blob hit is not accepted only because the expected tag exists. The remote image is also validated to ensure that:

- it contains exactly one layer, and
- the layer digest matches the expected compressed blob digest.

This protects the cache against incorrect or manually overwritten tags.

## Current State and Build Integration

The current implementation provides the main standalone remote-cache primitives for both blobs and keys.

### Blob flow

The builder currently integrates remote blob handling into `buildAllotment`:

1. Local cache metadata is checked for a `compressedSha` / `diffID` mapping.
2. If a mapping exists, the builder checks whether the corresponding blob is available locally.
3. If the local blob is missing, the builder attempts to restore it from the remote cache.
4. A restored blob is streamed into the local cache while its SHA-256 digest is recomputed.
5. The restored blob is accepted only when the computed digest matches the expected `compressedSha` and the local cache integrity check succeeds.
6. If neither local nor remote blob is available, the allotment is rebuilt.
7. Once a valid local blob exists, `ensureRemoteBlob` publishes it to the remote cache when necessary.

Remote blob restore treats OCI `404 Not Found` as a normal cache miss. Other registry, stream, local write, close, or integrity errors are propagated as build errors. Partial local blobs are deleted after failed restores.

### Key flow

The key-level primitives are also implemented:

- deterministic `keyDigest` generation from `fileSha + dst`,
- remote key metadata construction,
- JSON encode/decode,
- semantic validation after pull,
- remote key pull helper,
- remote key publication helper,
- OCI registry `PushKey` / `PullKey` operations.

The remote key pull and publication helpers are currently **standalone primitives**. They are not yet connected to the final `buildAllotment` orchestration.

Therefore, the current build flow can restore a remote blob when a local key mapping already identifies that blob, but it does **not yet perform a remote key lookup after a local key miss**. Likewise, `buildAllotment` does not yet call the remote key publication helper after producing a new allotment.

### Local key upsert

The local key cache now performs a true upsert for destination mappings.

Previous behavior appended a new mapping even when the same destination already existed. Since local lookup returns the first matching destination, old mappings could remain reachable and produce stale cache resolution.

The current behavior is:

```text
same destination      → replace existing mapping
new destination       → append new mapping
existing duplicates   → collapse to one current mapping
```

This keeps local key metadata deterministic before the local and remote key flows are integrated.

## Implementation Components

### `cache/remote.go`

`cache/remote.go` contains the registry-backed remote cache implementation.

The `RemoteCache` interface currently defines five operations:

```text
CheckBlob(compressedSha)
PushBlob(compressedSha, reader)
PullBlob(compressedSha)
PushKey(keyDigest, reader)
PullKey(keyDigest)
```

#### Blob operations

`CheckBlob` resolves `blob-sha256-<compressedSha>` and validates the returned OCI image before accepting it as a hit.

`PushBlob` converts the compressed allotment blob into a single-layer OCI image and pushes it under the deterministic blob tag.

`PullBlob` resolves the same tag, validates the image, and returns a reader for the compressed layer. Missing entries are wrapped with `ErrRemoteCacheMiss`.

#### Key operations

`PushKey` converts the encoded remote key metadata into a single-layer OCI image and pushes it under:

```text
key-sha256-<keyDigest>
```

`PullKey` resolves the same tag, validates that the key image contains exactly one layer, and returns the layer reader. Missing entries are wrapped with `ErrRemoteCacheMiss`.

#### Registry configuration

`NewRemoteCache` configures the registry host, repository, insecure option, and Docker-keychain authentication. This allows the implementation to reuse credentials available through the default Docker credential configuration.

### `oci/remote_key.go`

`oci/remote_key.go` contains key identity, encoding, decoding, and semantic validation helpers.

`remoteKeyDigest` computes a deterministic SHA-256 digest from JSON containing only `fileSha` and `dst`. `compressedSha` and `diffID` are deliberately excluded because they are the values resolved by the key rather than part of its lookup identity.

`newRemoteKey` creates the metadata payload:

```text
fileSha
+ dst
+ compressedSha
+ diffID
```

`encodeRemoteKey` validates the metadata and serializes it to JSON.

`decodeRemoteKey` decodes and validates pulled metadata.

`validateRemoteKeyMatch` verifies that pulled metadata still matches the requested `fileSha`, `dst`, and deterministic `keyDigest`.

### `oci/image.go`

`oci/image.go` contains the build-level remote cache helpers.

`newRemoteCacheFromContext` initializes the remote cache when registry and repository values are configured. If remote cache configuration is missing, remote caching remains disabled and the existing local-only behavior is preserved.

`restoreRemoteBlob` restores a remote blob into the local cache and verifies the downloaded bytes against the expected compressed SHA-256 digest. Failed or partial restores are removed.

`ensureRemoteBlob` checks the remote registry and publishes a local compressed blob only when it is not already available remotely.

`lookupRemoteKey` computes the semantic key digest, pulls the corresponding key metadata, decodes it, and validates that it matches the requested identity. A remote cache miss is returned as a normal miss; malformed metadata and registry errors are returned as failures.

`publishRemoteKey` computes the semantic key digest, constructs and encodes the remote key metadata, and calls `PushKey`. If no remote cache is configured, the operation is a no-op. The helper is implemented and tested but is not yet called from `buildAllotment`.

The local `upsertLocalCacheKey` path now uses destination-aware upsert behavior instead of append-only metadata updates.

### `cmd/build.go`

The build command exposes remote cache configuration through:

```text
--remote-cache-registry
--remote-cache-repository
--remote-cache-insecure
```

The feature remains opt-in. Without valid remote-cache registry and repository configuration, the builder does not initialize a remote cache.

`--force-http` remains separate from `--remote-cache-insecure`: the former belongs to the existing image pull/push flow, while the latter controls insecure access specifically for the remote cache registry.

## Validation

### Core remote cache tests

`cache/remote_test.go` covers the registry-facing remote cache implementation, including:

- deterministic blob and key tag generation,
- digest normalization,
- blob and key reference construction,
- single-layer blob image creation,
- blob digest validation and rejection of mismatches,
- reader failure handling,
- blob push/pull behavior against an in-process registry,
- key push/pull behavior against an in-process registry,
- `ErrRemoteCacheMiss` behavior for missing blob/key entries,
- verification that key metadata is stored under the expected deterministic key tag.

Command:

```bash
go test -count=1 ./cache -v
```

### Remote cache context tests

`oci/remote_cache_context_test.go` verifies that remote cache configuration:

- stays disabled when values are missing or incomplete,
- becomes enabled with valid registry/repository values,
- rejects invalid context value types.

Command:

```bash
go test -count=1 ./oci -run 'TestNewRemoteCacheFromContext' -v
```

### Remote blob restore tests

`oci/remote_blob_restore_test.go` verifies:

- successful restoration of a matching remote blob,
- rejection and cleanup after digest mismatch,
- propagation of mid-stream restore errors and deletion of the partial local blob.

### Remote key tests

`oci/remote_key_test.go` verifies the pure key helpers, including:

- deterministic key digest generation,
- digest changes when semantic identity changes,
- rejection of invalid identity,
- metadata construction,
- encode/decode round trips,
- invalid JSON and missing-field rejection,
- semantic identity validation.

`oci/remote_key_orch_test.go` covers the `containerImage` remote-key orchestration helpers.

Pull tests verify:

- validated remote key hits,
- normal cache misses,
- registry error propagation,
- invalid JSON rejection,
- semantic identity mismatch rejection,
- nil-reader rejection,
- disabled-remote-cache behavior.

Push tests verify:

- `PushKey` is called exactly once for a publication,
- the deterministic expected `keyDigest` is used,
- the published payload contains the expected `fileSha`, `dst`, `compressedSha`, and `diffID`,
- remote push errors are propagated,
- publication is a no-op when no remote cache is configured.

A focused orchestration command is:

```bash
go test -count=1 ./oci -run 'Test(Lookup|Publish)RemoteKey' -v
```

### Local key upsert tests

The local key upsert tests verify that:

- an existing destination is replaced by its new mapping,
- a new destination is appended without changing unrelated mappings,
- duplicate mappings for the same destination are collapsed while unrelated destinations remain unchanged.

## Existing Registry Validation

The earlier blob-cache MVP was manually validated against both a local OCI registry and Docker Hub.

The local-registry tests demonstrated that compressed allotment blobs are published under deterministic `blob-sha256-...` tags on the first build and that subsequent builds skip uploading blobs already present in the cache repository.

Docker Hub validation demonstrated that the same blob representation and deterministic tag scheme work with a standard external OCI registry.

These manual validations cover the blob push/check path. The newer remote key helpers and final remote-key build orchestration should receive their own end-to-end validation after the key pull/publication helpers are connected to `buildAllotment`.

## Remaining Integration Work

The standalone blob and key primitives are implemented. The main remaining work is build-flow orchestration and hardening.

### Build-flow integration

The intended flow is:

```text
local key lookup
    ↓
try local/remote blob for local mapping
    ↓
if unresolved, remote key lookup
    ↓
try local/remote blob for remote mapping
    ↓
verified blob → hydrate local key
    ↓
otherwise rebuild
```

After a rebuild, the intended publication order is:

```text
local blob ready
    ↓
PushBlob
    ↓
PushKey
```

The key must not be published before its referenced blob is available remotely.

The following changes remain pending architectural review/integration:

- extract the current local key lookup block from `buildAllotment`,
- replace `log.Fatal` in local key parsing with controlled error propagation,
- enforce local blob-before-key write ordering,
- integrate remote key lookup into `buildAllotment`,
- hydrate the local key only after the resolved blob is available and verified,
- integrate remote key publication into `buildAllotment` after successful blob publication.

### Later hardening

Further hardening remains outside the current integration step:

- retry/backoff policy for `429`, transient `5xx`, and network failures,
- corruption retry policy,
- stricter malformed/trailing JSON handling,
- safe cleanup/deletion semantics for corrupted remote entries,
- concurrent-worker race tests,
- cache lifecycle/cleanup policy,
- performance metrics and distributed end-to-end evaluation.
