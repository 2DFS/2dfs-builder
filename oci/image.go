package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"log"

	"github.com/giobart/2dfs-builder/cache"
	"github.com/giobart/2dfs-builder/filesystem"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// DefaultRegistry is the default registry to use
	DefaultRegistry = "index.docker.io"
	// IndexStoreContextKey is the context key for the index store
	IndexStoreContextKey = "indexStore"
	// BlobStoreContextKey is the context key for the blob store
	BlobStoreContextKey = "blobStore"
)

type containerImage struct {
	index      v1.Index
	registry   string
	repository string
	tag        string
	indexCache cache.CacheStore
	blobCache  cache.CacheStore
	manifests  []v1.Manifest
}

type Image interface {
	AddField(fs filesystem.Field) error
}

func NewImage(url string, ctx context.Context) (Image, error) {

	ctxIndexPosition := ctx.Value(IndexStoreContextKey)
	indexStoreLocation := ""
	if ctxIndexPosition != nil {
		indexStoreLocation = ctxIndexPosition.(string)
	} else {
		return nil, fmt.Errorf("Index store location not found in context")
	}

	ctxBlobPosition := ctx.Value(BlobStoreContextKey)
	blobStoreLocation := ""
	if ctxBlobPosition != nil {
		blobStoreLocation = ctxBlobPosition.(string)
	} else {
		return nil, fmt.Errorf("Blob store location not found in context")
	}

	imgstore, err := cache.NewCacheStore(indexStoreLocation)
	if err != nil {
		return nil, err
	}
	blobstore, err := cache.NewCacheStore(blobStoreLocation)
	if err != nil {
		return nil, err
	}

	img := &containerImage{
		indexCache: imgstore,
		blobCache:  blobstore,
		manifests:  []v1.Manifest{},
	}

	err = img.loadIndex(url, ctx)
	if err != nil {
		return nil, err
	}

	err = img.downloadManifests()
	if err != nil {
		return nil, err
	}

	for _, manifest := range img.manifests {
		err = img.downloadManifestBlobs(manifest)
		if err != nil {
			return nil, err
		}
	}

	return img, nil

}

func (c *containerImage) loadIndex(url string, ctx context.Context) error {
	// if path is an URL use Distribution spec to download image index
	// if path is a local file use fsutil.ReadFile

	//check index local cache first

	indexReader, err := c.indexCache.Get(url)
	if err == nil {
		// load index from cache
		defer indexReader.Close()
		index, err := ReadIndex(indexReader)
		if err != nil {
			return err
		}
		c.index = index
		return nil
	}

	// download image online
	urlParts := strings.SplitN(url, "/", 2)
	registryRegex := regexp.MustCompile(`\b([a-z]+)\.?\s*(?:\b([a-z]+)\.?\s*)*`)
	registry := urlParts[0]
	repo := fmt.Sprintf(urlParts[1])
	if registryRegex.FindStringIndex(registry) == nil {
		registry = "index.docker.io"
		repo = url
	}

	tagAndRepo := strings.Split(repo, ":")
	tag := "latest"
	if len(tagAndRepo) == 2 {
		tag = tagAndRepo[1]
		repo = tagAndRepo[0]
	}

	index, err := DownloadIndex(OciImageLink{
		Registry:   registry,
		Repository: repo,
		Tag:        tag,
	})
	if err != nil {
		return err
	}
	c.registry = registry
	c.repository = repo
	c.tag = tag

	// save index to cache
	uploadWriter, err := c.indexCache.Add(url)
	if err != nil {
		return err
	}
	defer uploadWriter.Close()
	indexBytes, err := json.Marshal(index)
	if err != nil {
		return err
	}
	_, err = uploadWriter.Write(indexBytes)
	if err != nil {
		return err
	}

	c.index = index
	return nil
}

func (c *containerImage) AddField(fs filesystem.Field) error {
	return nil
}

func (c *containerImage) downloadManifests() error {
	for _, manifest := range c.index.Manifests {

		if manifest.Digest.Algorithm() != digest.SHA256 {
			return fmt.Errorf("unsupported digest algorithm: %s", manifest.Digest.Algorithm().String())
		}

		// check if blob cached
		_, err := c.blobCache.Get(manifest.Digest.Encoded())
		if err == nil {
			// blob already cached, continue
			log.Printf("%s [CACHED]", manifest.Digest.Encoded())
			continue
		}

		// download blob
		c.downloadAndCache(manifest.Digest)

		// read manifest from cache and update container struct
		manifestCachereader, err := c.blobCache.Get(manifest.Digest.Encoded())
		if err != nil {
			return err
		}
		defer manifestCachereader.Close()
		manifest, err := ReadManifest(manifestCachereader)
		if err != nil {
			return err
		}
		c.manifests = append(c.manifests, manifest)

	}
	return nil
}

func (c *containerImage) downloadManifestBlobs(manifest v1.Manifest) error {
	// download config blob
	if manifest.Config.Digest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("unsupported digest algorithm: %s", manifest.Config.Digest.Algorithm().String())
	}
	_, err := c.blobCache.Get(manifest.Config.Digest.Encoded())
	if err != nil {
		// blob not cached cached, downloading
		c.downloadAndCache(manifest.Config.Digest)

	} else {
		log.Printf("%s [CACHED]", manifest.Config.Digest.Encoded())
	}

	// download layers
	for _, layer := range manifest.Layers {
		if layer.Digest.Algorithm() != digest.SHA256 {
			return fmt.Errorf("unsupported digest algorithm: %s", layer.Digest.Algorithm().String())
		}
		_, err := c.blobCache.Get(layer.Digest.Encoded())
		if err != nil {
			// blob not cached cached, downloading
			// TODO: this can be parallelized!
			err := c.downloadAndCache(layer.Digest)
			if err != nil {
				log.Printf("Error downloading layer: %v", err)
			}
		} else {
			log.Printf("%s [CACHED]", layer.Digest.Encoded())
		}
	}
	return nil
}

func (c *containerImage) downloadAndCache(downloadDigest digest.Digest) error {
	if downloadDigest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("unsupported digest algorithm: %s", downloadDigest.Algorithm().String())
	}

	log.Printf("%s [DOWNLOADING]", downloadDigest.Encoded())
	manifestReader, err := DownloadBlob(
		OciImageLink{
			Registry:   c.registry,
			Repository: c.repository,
			Tag:        c.tag,
		},
		downloadDigest,
	)
	if err != nil {
		return err
	}
	defer manifestReader.Close()

	// upload blob to cache store
	uploadWriter, err := c.blobCache.Add(downloadDigest.Encoded())
	if err != nil {
		return err
	}
	_, err = io.Copy(uploadWriter, manifestReader)
	if err != nil {
		return err
	}
	defer uploadWriter.Close()

	if !c.blobCache.Check(downloadDigest.Encoded()) {
		c.blobCache.Del(downloadDigest.Encoded())
		return fmt.Errorf("blob integrity check failed, please retry")
	}

	return nil
}
