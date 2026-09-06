    # Remote Cache - Current Implementation

    > **Status:** Current implementation state as of September 2026.
    >
    > Historical implementation snapshots are preserved in `remote-cache-v1.md` and `remote-cache-v2.md`.

    ## Scope

    This document describes the current OCI registry-backed remote cache implementation of the 2DFS builder.

    The remote cache extends the existing local 2DFS cache so that cache state can be reused across machines, stateless CI/CD workers, edge devices, and other ephemeral build environments.

    The implementation remains client-side and uses standard OCI registry operations. No remote-cache-specific registry modification or central mutable cache index is required.

    ## Architecture Summary

    The remote cache uses two independently addressable cache entities:

    ```text
    key-sha256-<keyDigest>
    blob-sha256-<compressedSha>
    ```

    They serve different purposes:

    - **Remote blob:** stores the compressed allotment blob and is addressed by its compressed SHA-256 digest.
    - **Remote key:** maps the semantic identity of an allotment to the corresponding compressed blob and DiffID.

    The remote key identity is derived from:

    ```text
    fileSha + dst
        ↓
    SHA-256(JSON({fileSha, dst}))
        ↓
    keyDigest
    ```

    The remote key payload contains:

    ```json
    {
    "fileSha": "...",
    "dst": ["..."],
    "compressedSha": "...",
    "diffID": "..."
    }
    ```

    This separates semantic lookup identity from blob identity. A worker can determine which compressed blob belongs to an allotment before rebuilding that allotment.

    ## OCI Registry Representation

    Both key and blob entries are stored as single-layer OCI images.

    ### Blob entry

    ```text
    blob-sha256-<compressedSha>
        ↓
    single OCI layer
        ↓
    compressed allotment blob
    ```

    The blob image is validated before it is accepted as a cache hit. The implementation verifies that:

    - the image contains exactly one layer, and
    - the layer digest matches the expected compressed blob digest.

    ### Key entry

    ```text
    key-sha256-<keyDigest>
        ↓
    single OCI layer
        ↓
    RemoteKey JSON metadata
    ```

    Pulled remote keys are decoded and semantically validated against the requested:

    ```text
    fileSha
    dst
    keyDigest
    ```

    before their blob mapping is trusted.

    ## Current Build Flow

    Remote cache handling is integrated into `containerImage.buildAllotment`.

    The current flow is:

    ```text
    calculate fileSha
            ↓
    local key lookup
            ↓
    usable local key + local blob?
        ┌───┴────┐
    yes       no
        │         ↓
        │   remote key lookup
        │         ↓
        │   remote key hit?
        │     ┌───┴────┐
        │    yes       no
        │     │         │
        │     ↓         │
        │ remote blob   │
        │   restore     │
        │     ↓         │
        │ digest and    │
        │ local cache   │
        │ validation    │
        │     ↓         │
        │ hydrate       │
        │ local key     │
        │     │         │
        └─────┴─────────┘
            ↓
    unresolved?
        ┌────┴────┐
        yes        no
        │          │
        ↓          │
    rebuild        │
    allotment      │
        └────┬─────┘
            ↓
    ensure remote blob
            ↓
    publish remote key
            ↓
    add allotment to field
    ```

    ### Local cache lookup

    The builder first calculates `fileSha` from the allotment source files and checks the local key cache.

    If a valid local mapping exists and its compressed blob is present in the local blob cache, the existing local cache path is used.

    If a local key points to a missing local blob, that mapping is treated as unusable and the build proceeds to the remote key lookup path.

    ### Remote key lookup

    If there is no usable local cache entry, the builder derives the deterministic remote `keyDigest` from:

    ```text
    fileSha + dst
    ```

    and calls `lookupRemoteKey`.

    A missing remote key is treated as a normal cache miss. Registry failures, malformed metadata, and semantic validation failures are propagated as errors.

    ### Remote blob restore

    When a valid remote key is found, the builder attempts to restore the referenced compressed blob.

    `restoreRemoteBlob`:

    1. pulls the compressed layer from the remote cache,
    2. streams it into the local blob cache,
    3. recomputes its SHA-256 digest while streaming,
    4. compares the computed digest with the expected `compressedSha`,
    5. verifies the resulting local cache entry,
    6. removes partial or invalid local blobs when restoration fails.

    Only after the blob has been successfully restored and validated is the local key cache hydrated with the remote mapping.

    ### Rebuild fallback

    If neither the local cache nor the remote cache resolves the allotment, the normal 2DFS allotment build path is used.

    The builder creates the tar archive, calculates the DiffID, compresses the allotment, calculates `compressedSha`, and stores the result in the local cache.

    ## Remote Publication

    Once a valid local compressed blob is available, the builder synchronizes the result with the remote cache.

    The publication order is intentionally:

    ```text
    local blob available
            ↓
    ensureRemoteBlob
            ↓
    remote blob available
            ↓
    publishRemoteKey
    ```

    The remote key is therefore published only after its referenced blob is available in the remote registry.

    `ensureRemoteBlob` first checks whether the deterministic blob entry already exists. Existing valid blobs are reused without another push.

    `publishRemoteKey` then publishes the semantic mapping:

    ```text
    fileSha + dst
            ↓
    compressedSha + diffID
    ```

    under its deterministic key tag.

    ## Local Key Upsert

    Local key metadata uses destination-aware upsert behavior.

    For a given destination:

    ```text
    existing destination    → replace mapping
    new destination         → append mapping
    duplicate destinations  → collapse to one current mapping
    ```

    This prevents stale destination mappings from remaining reachable through the local key cache.

    ## Remote Cache Configuration

    The build command exposes:

    ```text
    --remote-cache-registry
    --remote-cache-repository
    --remote-cache-insecure
    ```

    Example:

    ```text
    --remote-cache-registry localhost:5000
    --remote-cache-repository 2dfs/cache
    --remote-cache-insecure
    ```

    Remote caching is opt-in.

    If registry or repository configuration is missing or empty, no remote cache is initialized and the existing local-only behavior is preserved.

    `--remote-cache-insecure` applies only to remote-cache registry access and remains separate from the existing `--force-http` image transport option.

    Registry authentication uses the Docker credential keychain used by the OCI client.

    ## Implementation Map

    ### `cache/remote.go`

    Provides the registry-facing `RemoteCache` implementation:

    ```text
    CheckBlob
    PushBlob
    PullBlob
    PushKey
    PullKey
    ```

    It is responsible for:

    - deterministic key/blob tags,
    - OCI reference construction,
    - registry access,
    - OCI image creation,
    - blob validation,
    - key image validation,
    - remote-cache miss classification.

    ### `oci/remote_key.go`

    Provides remote-key identity and metadata handling:

    ```text
    remoteKeyDigest
    newRemoteKey
    encodeRemoteKey
    decodeRemoteKey
    validateRemoteKeyIdentity
    validateRemoteKeyMatch
    validateRemoteKey
    ```

    ### `oci/image.go`

    Contains build orchestration:

    ```text
    newRemoteCacheFromContext
    lookupRemoteKey
    restoreRemoteBlob
    ensureRemoteBlob
    publishRemoteKey
    upsertLocalCacheKey
    ```

    and integrates these operations into `buildAllotment`.

    ### `filesystem/types.go`

    Defines the remote key payload:

    ```go
    type RemoteKey struct {
        FileSha       string   `json:"fileSha"`
        Dst           []string `json:"dst"`
        CompressedSha string   `json:"compressedSha"`
        DiffID        string   `json:"diffID"`
    }
    ```

    ### `cmd/build.go`

    Exposes remote-cache configuration to the CLI and passes it into the OCI build layer.

    ## Validation

    The remote cache implementation currently has focused tests for:

    - blob and key tag normalization,
    - OCI reference construction,
    - single-layer blob creation,
    - blob digest validation,
    - blob push and pull against an in-process registry,
    - key push and pull against an in-process registry,
    - remote-cache miss handling,
    - remote cache context initialization,
    - successful remote blob restoration,
    - restore digest mismatch and partial-write cleanup,
    - remote key lookup and publication,
    - remote key encoding and semantic validation,
    - local key destination-aware upsert behavior.

    Relevant test files include:

    ```text
    cache/remote_test.go
    oci/remote_cache_integration_test.go
    oci/remote_key_test.go
    oci/local_cache_key_test.go
    ```

    The core remote-cache package can be validated with:

    ```bash
    go test -count=1 ./cache
    ```

    The full `oci` package currently contains an external Docker registry test (`TestDownloadIndexDocker`) that may fail independently with HTTP `429 Too Many Requests`. Such a failure is unrelated to the local remote-cache test flow.

    ## Known Follow-up Work

    The current implementation is functionally integrated, but several production-hardening items remain intentionally outside the maintenance cleanup phase:

    - replace process-level `log.Fatal` behavior in the local key parsing path with controlled error propagation,
    - propagate the currently ignored local-key upsert error in the rebuild path,
    - review local blob/key write ordering during freshly rebuilt allotments,
    - tighten malformed or trailing JSON handling for remote key metadata,
    - review temporary-file close and cleanup error handling,
    - evaluate a lighter-weight remote blob existence check,
    - define retry/backoff behavior for `429`, transient `5xx`, and network failures,
    - add concurrent-worker and distributed end-to-end validation,
    - define remote cache lifecycle and cleanup policy,
    - perform performance evaluation against the local-cache baseline.

    These items should be handled as explicit correctness, hardening, or performance changes rather than being mixed into maintenance-only refactoring.

    ## Historical Documentation

    Earlier implementation states are preserved for design history:

    - `remote-cache-v1.md` — initial remote blob cache MVP, June 2026.
    - `remote-cache-v2.md` — intermediate remote key integration design, August 2026.

    This document should be treated as the current implementation reference.
