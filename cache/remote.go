package cache

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var ErrRemoteCacheMiss = errors.New("remote cache entry not found")

type RemoteCache interface {
	CheckBlob(compressedSha string) (bool, error)
	PushBlob(compressedSha string, r io.Reader) error
	PullBlob(compressedSha string) (io.ReadCloser, error)

	PushKey(keyDigest string, reader io.Reader) error
	PullKey(keyDigest string) (io.ReadCloser, error)
}

type remoteCache struct {
	registryURL string
	repository  string
	insecure    bool
	options     []remote.Option
}

func NewRemoteCache(registryURL, repository string, insecure bool) RemoteCache {
	return &remoteCache{
		registryURL: registryURL,
		repository:  repository,
		insecure:    insecure,
		options: []remote.Option{
			remote.WithAuthFromKeychain(authn.DefaultKeychain),
		},
	}
}

func (c *remoteCache) CheckBlob(compressedSha string) (bool, error) {
	ref, err := c.reference(blobTag(compressedSha), "blob")
	if err != nil {
		return false, err
	}

	img, err := remote.Image(ref, c.options...)
	if err != nil {
		if isRemoteCacheMiss(err) {
			return false, nil
		}
		return false, fmt.Errorf("Failed to fetch blob from remote cache: %w", err)
	}

	err = c.validateBlobImage(img, compressedSha)
	if err != nil {
		return false, fmt.Errorf("Invalid blob image in remote cache: %w", err)
	}

	return true, nil
}

// Get tar.gz allotment blob from the reader and push it to the registry as a single-layer OCI image
func (c *remoteCache) PushBlob(compressedSha string, r io.Reader) error {
	img, cleanup, err := c.createBlobImage(compressedSha, r)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	ref, err := c.reference(blobTag(compressedSha), "blob")
	if err != nil {
		return err
	}

	err = remote.Write(ref, img, c.options...)
	if err != nil {
		return fmt.Errorf("Failed to push blob image to remote cache: %w", err)
	}

	return nil
}

func (c *remoteCache) createBlobImage(compressedSha string, r io.Reader) (v1.Image, func(), error) {
	tempFile, err := os.CreateTemp("", "2dfs-remote-blob-*.tar.gz")
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create temporary file for blob: %w", err)
	}

	cleanup := func() {
		os.Remove(tempFile.Name())
	}

	_, err = io.Copy(tempFile, r)
	if err != nil {
		tempFile.Close()
		cleanup()
		return nil, nil, fmt.Errorf("Failed to write blob data to temporary file: %w", err)
	}

	err = tempFile.Close()
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("Failed to close temporary file: %w", err)
	}

	layer, err := tarball.LayerFromFile(
		tempFile.Name(),
		tarball.WithMediaType(types.OCILayer),
	)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("Failed to create layer from temporary blob file: %w", err)
	}

	err = validateLayerDigest(layer, compressedSha)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("Failed to create blob image with OCI layer: %w", err)
	}

	return img, cleanup, nil
}

func isRemoteCacheMiss(err error) bool {
	var transportErr *transport.Error
	return errors.As(err, &transportErr) && transportErr.StatusCode == http.StatusNotFound
}

func validateLayerDigest(layer v1.Layer, expectedDigest string) error {
	layerDigest, err := layer.Digest()
	if err != nil {
		return fmt.Errorf("Failed to calculate OCI layer digest: %w", err)
	}

	expectedHex := normalizeHexDigest(expectedDigest)
	if layerDigest.Algorithm != "sha256" || !strings.EqualFold(layerDigest.Hex, expectedHex) {
		return fmt.Errorf("Layer digest mismatch: expected sha256:%s, got %s", expectedHex, layerDigest.String())
	}

	return nil
}

func (c *remoteCache) PullBlob(compressedSha string) (io.ReadCloser, error) {
	ref, err := c.reference(blobTag(compressedSha), "blob")
	if err != nil {
		return nil, err
	}

	img, err := remote.Image(ref, c.options...)
	if err != nil {
		if isRemoteCacheMiss(err) {
			return nil, fmt.Errorf("Blob %s not found in remote cache: %w", compressedSha, ErrRemoteCacheMiss)
		}
		return nil, fmt.Errorf("Failed to fetch blob from remote cache: %w", err)
	}

	err = c.validateBlobImage(img, compressedSha)
	if err != nil {
		return nil, fmt.Errorf("Invalid blob image in remote cache: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("Failed to get blob layers: %w", err)
	}

	reader, err := layers[0].Compressed()
	if err != nil {
		return nil, fmt.Errorf("Failed to open compressed blob layer from remote cache: %w", err)
	}

	return reader, nil
}

func (c *remoteCache) PushKey(keyDigest string, reader io.Reader) error {
	img, err := createKeyImage(reader)
	if err != nil {
		return err
	}

	ref, err := c.reference(keyTag(keyDigest), "key")
	if err != nil {
		return err
	}

	err = remote.Write(ref, img, c.options...)
	if err != nil {
		return fmt.Errorf("Failed to push key image to remote cache: %w", err)
	}

	return nil
}

func (c *remoteCache) PullKey(keyDigest string) (io.ReadCloser, error) {
	ref, err := c.reference(keyTag(keyDigest), "key")
	if err != nil {
		return nil, err
	}

	img, err := remote.Image(ref, c.options...)
	if err != nil {
		if isRemoteCacheMiss(err) {
			return nil, fmt.Errorf("Key %s not found in remote cache: %w", keyDigest, ErrRemoteCacheMiss)
		}
		return nil, fmt.Errorf("Failed to fetch key from remote cache: %w", err)
	}

	err = c.validateKeyImage(img)
	if err != nil {
		return nil, fmt.Errorf("Invalid key image in remote cache: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("Failed to get key layers: %w", err)
	}

	reader, err := layers[0].Compressed()
	if err != nil {
		return nil, fmt.Errorf("Failed to open compressed key layer from cache: %w", err)
	}

	return reader, nil
}

func createKeyImage(reader io.Reader) (v1.Image, error) {
	// ReadAll is sufficient because the expected key metadata is a small JSON payload
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("Failed to read key data: %w", err)
	}

	layer := static.NewLayer(data, types.OCILayer)

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return nil, fmt.Errorf("Failed to create key image with OCI layer: %w", err)
	}

	return img, nil
}

func keyTag(keyDigest string) string {
	return "key-sha256-" + normalizeHexDigest(keyDigest)
}

func blobTag(compressedSha string) string {
	return "blob-sha256-" + normalizeHexDigest(compressedSha)
}

func normalizeHexDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	digest = strings.TrimPrefix(digest, "sha256:")
	digest = strings.TrimPrefix(digest, "sha256-")
	return digest
}

func (c *remoteCache) reference(tag, entryType string) (name.Reference, error) {
	registryURL := strings.TrimSuffix(c.registryURL, "/")
	repository := strings.TrimPrefix(c.repository, "/")

	target := fmt.Sprintf("%s/%s:%s", registryURL, repository, tag)

	var opts []name.Option
	if c.insecure {
		opts = append(opts, name.Insecure)
	}

	ref, err := name.ParseReference(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse remote %s reference: %q: %w", entryType, target, err)
	}

	return ref, nil
}

func (c *remoteCache) validateBlobImage(img v1.Image, expectedCompressedSha string) error {
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("Failed to get blob layers: %w", err)
	}

	if len(layers) != 1 {
		return fmt.Errorf("Expected exactly one layer in the blob image, but found %d", len(layers))
	}

	return validateLayerDigest(layers[0], expectedCompressedSha)
}

func (c *remoteCache) validateKeyImage(img v1.Image) error {
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("Failed to get key layers: %w", err)
	}

	if len(layers) != 1 {
		return fmt.Errorf("Expected exactly one layer in the key image, but found %d", len(layers))
	}

	return nil
}
