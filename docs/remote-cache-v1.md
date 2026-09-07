# remote-cache-v1
> **Status:** Historical MVP snapshot from June 15, 2026. This document is kept for design history and is superseded by `remote-cache-current.md`.

## Scope

This document describes the design and current implementation status of the remote blob cache MVP for the 2DFS builder.

The broader goal of the project is to make 2DFS cache data reusable across different machines and build environments. Existing cache mechanism works locally; its state is tied to a single machine. Using an OCI-compatible registry as a remote cache enables cache sharing in distributed or stateless environments such as CI/CD runners, edge devices, and ephemeral workers.

## Design Summary

Every cache entry in the remote cache represents one 2DFS allotment. This allows the system to operate at the level of the smallest entity of the 2DFS filesystem. The flow aims to achieve 3 goals when working with a remote cache:

1. Skip uploading already stored allotment blobs.
2. Skip building allotments that are already available in the remote cache.
3. Hold reusable cache state in a remote OCI registry for ephemeral or edge devices.

Remote Cache is simply a repository in the OCI registry that is different from the target repository for the final images similar to the `.blobcache`, `.indexcache` utilized by the local cache.

We use two types of cache entities in the remote cache:

```text
key-sha256-<compressedSha>
blob-sha256-<compressedSha>
```

The remote cache object is addressed by the compressed blob digest. The digest is converted into a deterministic OCI tag.

The current MVP focuses on the blob-level entity. The key-level entity is part of the intended design for later remote build-skip and restore flows.

This design keeps the remote cache stateless from the builder's perspective. Instead of maintaining a central JSON index or a mutable cache manifest, each blob can be checked independently by looking up its deterministic tag in the registry.

### Single-layer OCI image per compressed blob

Each cached allotment blob is wrapped as a single-layer OCI image. The original compressed blob becomes the only layer of that image. This keeps the representation compatible with standard OCI registries and avoids requiring changes to the registry backend.

Main goal of this decision is to make remote-cache compatible with the already existing registry technologies such as Docker Hub, Harbor etc.

### Digest-based tag scheme

The compressed blob digest is used as the cache identity. This makes the remote cache content-addressed from the builder's perspective: if two builds produce the same compressed allotment blob, they resolve to the same remote cache tag.

This keeps lookup cost independent from the number of stored cache entries, because the builder performs a direct tag lookup instead of listing or scanning the repository.

### Separate cache repository

The remote cache can be stored in a separate repository from the final target images. This keeps cache artifacts isolated from normal image tags and makes the registry layout easier to inspect and manage. Additionally, the repository that the user will operate on is prevented from the cache clutter that will build up as the images are created, updated and deployed.

Example layout:

```text
localhost:5000/2dfs/cache:blob-sha256-...
localhost:5000/2dfs/my-image:v1
```

### Tag per Blob Cache Entry Separation

A central mutable index or manifest list for cache metadata, such as a JSON file, is avoided intentionally. Such an index would require read-modify-write coordination between distributed builders and could introduce race conditions when multiple workers push cache entries concurrently.

The current tag-per-blob design avoids this problem by making each remote cache entry independently addressable. It also avoids relying on registry garbage collection behavior for shared index objects. Since cache entries are stored as independent OCI images referenced by deterministic tags, removing or updating one cache entry does not require rewriting a central metadata object that could become a garbage collection concern or a coordination bottleneck.

### Validation during CheckBlob

Checking whether a remote blob exists is not limited to checking whether the tag is present. The implementation also validates that the remote image has exactly one layer and that the layer digest matches the expected compressed blob digest.

This prevents accepting an incorrect or corrupted remote cache entry only because a tag with the expected name exists. Since tags are manually overridable this extra checking step is aimed at ensuring cache consistency.

## Current State and Build Integration

The current implementation focuses on the blob-level remote cache flow. In this version, the builder can check whether a compressed allotment blob already exists in a remote OCI registry and push the blob when it is missing.

1. The 2DFS builder creates or retrieves, from the local cache, compressed allotment blobs during the normal build process.
2. For each compressed allotment blob, the builder computes or receives its compressed digest.
3. The remote cache checks whether an OCI image tagged as `blob-sha256-<compressedSha>` already exists in the configured remote cache repository.
4. If the remote cache entry already exists and passes validation, the builder skips the remote push.
5. If the remote cache entry does not exist, the builder wraps the compressed blob as a single-layer OCI image and pushes it to the remote cache repository.

The current implementation is more useful for the first build operation for a given 2DFS image and does not yet pull missing local blobs from the remote cache and perform key-level remote build skipping. These parts are planned as backlog entries in the next steps.

### 3.1 `cache/remote.go`

The `cache/remote.go` file contains the core remote blob cache implementation. It defines the remote cache interface, constructs deterministic OCI references for compressed allotment blobs, checks whether a blob already exists in the remote registry, validates existing remote entries, and pushes missing blobs as single-layer OCI images.

The **`RemoteCache` interface** defines three operations:

* `CheckBlob(compressedSha)` checks whether a compressed allotment blob exists in the remote cache.
* `PushBlob(compressedSha, reader)` pushes a compressed allotment blob to the remote cache.
* `PullBlob(compressedSha)` is reserved for the planned remote restore flow and is not implemented in the current MVP.

**`NewRemoteCache`** creates a registry-backed remote cache instance. It stores the registry host, repository path, insecure registry option, and remote registry options. Registry authentication is configured through Docker keychain authentication, which allows the implementation to reuse credentials such as Docker Hub login credentials.

**`CheckBlob`** is used to detect whether a compressed allotment blob is already available in the remote cache. It constructs the deterministic OCI reference for the blob and tries to resolve the corresponding image from the registry.

If the registry returns `404 Not Found`, the function treats this as a cache miss. Other registry errors are returned as actual errors.

If the image exists, `CheckBlob` validates the image before accepting it as a cache hit. The validation checks that the image contains exactly one layer and that the layer digest matches the expected compressed blob digest. This avoids accepting an incorrect remote cache entry just because a tag with the expected name exists.

**`PushBlob`** takes a compressed allotment blob from an `io.Reader`, converts it into a single-layer OCI image, and pushes it to the deterministic `blob-sha256-<compressedSha>` tag in the configured remote cache repository.

The helper **`createBlobImage`** performs the OCI image construction. It writes the incoming blob stream into a temporary `.tar.gz` file, creates an OCI layer from that file, validates the layer digest, appends the layer to an empty OCI image, and returns the resulting single-layer image.

The temporary file is cleaned up only after the remote write operation completes, because the layer content can be read lazily during the push operation.

The helper functions `blobTag`, `normalizeHexDigest`, and `blobReference` provide deterministic tag and reference construction. They normalize digest strings with or without `sha256:` / `sha256-` prefixes and construct references such as:

```text
localhost:5000/2dfs/cache:blob-sha256-...
```

or:

```text
index.docker.io/alperp/seleniumk8test:blob-sha256-...
```

### 3.2 `cmd/build.go`

The `cmd/build.go` file exposes the remote cache configuration through build command flags. The following flags were added:

```text
--remote-cache-registry
--remote-cache-repository
--remote-cache-insecure
```

The registry flag specifies the remote registry host. The repository flag specifies the repository where remote cache entries are stored. The insecure flag allows the remote cache to use an HTTP registry, which is useful for local development registries such as `localhost:5000`.

These flags make the feature opt-in. If the remote cache flags are not provided, the builder keeps its existing behavior and does not initialize or use a remote cache.

The existing `--force-http` flag is separate from the remote cache flags. It belongs to the existing 2DFS image pull/push flow. The new `--remote-cache-insecure` flag only controls insecure access for the remote cache registry.

### 3.3 `oci/image.go`

The `oci/image.go` file contains the build-flow integration.

The remote cache configuration is passed from the command layer into the OCI image layer through context values. The implementation adds context keys for the remote cache registry, repository, and insecure flag.

A helper function initializes the remote cache from these context values. If the registry or repository is missing or empty, the helper returns `nil`, meaning that remote cache is disabled. If both values are present, it creates a new remote cache instance.

The `containerImage` structure was extended with a `remoteCache` field. This allows the build flow to access the remote cache while processing allotments.

The remote cache is integrated into the allotment build flow after a compressed allotment blob is available locally. At that point, the builder can check whether the blob already exists remotely. If it exists and passes validation, the remote push is skipped. If it does not exist, the local compressed blob is read from the local blob cache and pushed to the remote registry.

This keeps the remote cache integration non-invasive. The normal build process still produces or retrieves the compressed allotment blob locally first. The remote cache is then used as an additional sharing layer.

### 3.4 Tests and Dependencies

The remote cache MVP also adds tests and dependency updates.

The `cache/remote_test.go` file validates the core remote blob cache logic, including digest normalization, deterministic tag generation, blob reference construction, single-layer OCI image creation, wrong digest rejection, and reader error handling.

The `oci/remote_cache_context_test.go` file validates the context-based remote cache initialization logic. These tests ensure that remote cache stays disabled when configuration is missing or incomplete, becomes enabled when valid configuration is provided, and returns errors for invalid context value types.

The implementation uses `go-containerregistry` for OCI registry interactions. This avoids manually implementing registry API calls and keeps the implementation compatible with standard OCI registries.

## 4. Validation

The current MVP was validated with unit tests and manual end-to-end registry tests.

### 4.1 Unit Tests

The first group of tests validates the core remote blob cache logic in `cache/remote.go`.

Command:

```bash
go test -count=1 ./cache -v
```

This test group covers:

* deterministic `blob-sha256-...` tag generation,
* digest normalization,
* blob reference construction,
* single-layer OCI image creation,
* layer digest validation,
* wrong digest rejection,
* reader error handling.

The second group of tests validates the context-based remote cache initialization in `oci/image.go`.

Command:

```bash
go test -count=1 ./oci -run 'TestNewRemoteCacheFromContext' -v
```

This test group verifies that the remote cache remains disabled when configuration is missing or incomplete, becomes enabled when valid registry and repository values are provided, and fails explicitly for invalid context value types.

### 4.2 Local Registry Validation

The MVP was also tested against a local OCI registry running on `localhost:5000`.

The local registry validation uses the following setup:

```text
Base image: localhost:5000/base/alpine:latest
Target image: localhost:5000/2dfs/selenium-k8s-test:v-demo
Remote cache registry: localhost:5000
Remote cache repository: 2dfs/cache-demo-live
```

Example command:

```bash
tdfs build localhost:5000/base/alpine:latest localhost:5000/2dfs/selenium-k8s-test:v-demo \
  --force-http \
  --remote-cache-registry localhost:5000 \
  --remote-cache-repository 2dfs/cache-demo-live \
  --remote-cache-insecure
```

On the first run, the builder pushes the compressed allotment blobs to the remote cache repository.

Expected output pattern:

```text
Blob ... [PUSHED] to remote cache
```

The registry tags can be inspected with:

```bash
curl http://localhost:5000/v2/2dfs/cache-demo-live/tags/list
```

Expected result pattern:

```text
blob-sha256-...
blob-sha256-...
blob-sha256-...
blob-sha256-...
```

On the second run with the same cache repository, the builder detects that the blobs already exist in the remote cache and skips pushing them again.

Expected output pattern:

```text
Blob ... is already [CACHED] in remote registry
```

This validates the basic write-through and remote cache-hit behavior.

### 4.3 Docker Hub Validation

The MVP was also tested with Docker Hub as an external OCI registry.

Example command:

```bash
tdfs build localhost:5000/base/alpine:latest localhost:5000/2dfs/selenium-k8s-test:v-dockerhub-check \
  --force-http \
  --remote-cache-registry index.docker.io \
  --remote-cache-repository alperp/seleniumk8test
```

In this case, `--remote-cache-insecure` is not used because Docker Hub uses HTTPS.

This validation showed that the remote cache implementation is not limited to the local registry and can work with a standard external OCI registry. Docker Hub displayed the remote cache entries as tags using the deterministic `blob-sha256-...` format.

One practical observation from this test is that `index.docker.io` worked with the Docker keychain authentication, while `registry-1.docker.io` returned an authentication error in this setup. This may be handled later either through Docker Hub registry alias normalization or documentation.

## 5. Open Questions and Discussion Points

The current MVP leaves several design questions open for discussion.

### Remote Restore Flow

The current implementation can check and push remote blob cache entries, but it does not yet restore missing local blobs from the remote cache.

A future restore flow could work as follows:

1. Try to find the compressed blob in the local blob cache.
2. If it is missing locally but known by digest, check the remote cache.
3. If the remote blob exists, pull it from the registry.
4. Restore it into the local blob cache.
5. Continue the build using the restored blob.
6. If remote restore fails, fall back to the normal build path.

This would make the remote cache more useful for clean or ephemeral environments.

### Key-Level Remote Cache

The current MVP focuses only on blob-level cache entries. For clean runners, blob-level cache alone may not be enough because the builder also needs to know which compressed blob corresponds to a given allotment input.

A key-level remote cache could provide this mapping and enable stronger build-skip behavior.

### Docker Hub Registry Alias Handling

Docker Hub authentication may behave differently depending on whether the registry is addressed as `index.docker.io`, `docker.io`, or `registry-1.docker.io`.

The current implementation works with `index.docker.io` in the tested setup. A possible next step is to normalize Docker Hub aliases in the code or document the expected registry value clearly.

### Repository Layout

The current design uses a separate repository for remote cache entries. This keeps cache artifacts isolated from final image tags.

A remaining discussion point is whether the remote cache repository should always be separate, or whether some deployments may prefer storing cache entries under the same namespace as the final images.

### CI/CD Evaluation

The target use case includes CI/CD runners and ephemeral build workers. The next validation step should include a CI/CD pipeline, such as GitHub Actions or GitLab CI, to measure how the remote cache behaves in a clean build environment.

## 6. Next Steps

The next steps are:

1. Implement the remote restore flow using `PullBlob`.
2. Introduce or design key-level remote cache metadata for build-skip behavior.
3. Normalize or document Docker Hub registry naming behavior.
4. Add CI/CD pipeline validation.
5. Measure local cache vs remote cache performance.
6. Evaluate behavior with multiple builders pushing to the same remote cache repository.
7. Improve documentation and add example commands for local registry and Docker Hub usage.