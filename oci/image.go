package oci

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"log"

	"github.com/giobart/2dfs-builder/cache"
	compress "github.com/giobart/2dfs-builder/compress"
	"github.com/giobart/2dfs-builder/filesystem"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type contextKeyType string
type ManifestMediaType string

const (
	// DefaultRegistry is the default registry to use
	DefaultRegistry = "index.docker.io"
	// IndexStoreContextKey is the context key for the index store
	IndexStoreContextKey contextKeyType = "indexStore"
	// BlobStoreContextKey is the context key for the blob store
	BlobStoreContextKey contextKeyType = "blobStore"
	// 2dfs media type
	TwoDfsMediaType = "application/vnd.oci.image.layer.v1.2dfs.field"
	// image name annotation
	ImageNameAnnotation = "2dfs.image.name"
)

type containerImage struct {
	index      v1.Index
	registry   string
	repository string
	tag        string
	url        string
	indexCache cache.CacheStore
	blobCache  cache.CacheStore
	field      filesystem.Field
	manifests  []v1.Manifest
}

type Image interface {
	AddField(manifest filesystem.TwoDFsManifest, targetImage string) error
	GetExporter() FieldExporter
}

func NewImage(ctx context.Context, url string, forcepull bool) (Image, error) {

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

	// update container registry, tag and repository based on given url
	c.updateImageInfo(url)

	//check index local cache first
	log.Default().Println("Loading image index")

	indexReader, err := c.indexCache.Get(fmt.Sprintf("%x", sha256.Sum256([]byte(c.url))))
	if err == nil {
		log.Default().Printf("%s [CACHED] \n", c.url)
		// load index from cache
		defer indexReader.Close()
		index, err := ReadIndex(indexReader)
		if err != nil {
			// if error reading index, remove it from cache
			log.Default().Printf("unable to read %s from cache, removing it... try again please \n", url)
			c.indexCache.Del(fmt.Sprintf("%x", sha256.Sum256([]byte(c.url))))
			return err
		}
		c.index = index
		return nil
	}

	log.Default().Printf("[DOWNLOADING] %s \n", c.url)
	// download image online
	index, err := DownloadIndex(OciImageLink{
		Registry:   c.registry,
		Repository: c.repository,
		Tag:        c.tag,
	})
	if err != nil {
		return err
	}
	log.Default().Println("Index downloaded")

	// save index to cache
	uploadWriter, err := c.indexCache.Add(fmt.Sprintf("%x", sha256.Sum256([]byte(c.url))))
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

func (c *containerImage) updateImageInfo(url string) {
	urlParts := strings.SplitN(url, "/", 2)
	if len(urlParts) == 1 {
		c.registry = "docker.io"
		c.repository = url
	} else {
		registryRegex := regexp.MustCompile(`\b([a-z]+)\.?\s*(?:\b([a-z]+)\.?\s*)*`)
		registry := urlParts[0]
		c.repository = fmt.Sprintf(urlParts[1])
		if registryRegex.FindStringIndex(registry) == nil {
			c.registry = "docker.io"
		}
	}

	// add default library repo if not present
	if strings.Count(c.repository, "/") == 0 {
		c.repository = "library/" + c.repository
	}

	//check tag
	tagAndRepo := strings.Split(c.repository, ":")
	c.tag = "latest"
	if len(tagAndRepo) == 2 {
		c.tag = tagAndRepo[1]
		c.repository = tagAndRepo[0]
	}

	c.url = c.registry + "/" + c.repository + ":" + c.tag
}

func (c *containerImage) AddField(manifest filesystem.TwoDFsManifest, targetUrl string) error {

	fs, err := c.buildFiled(manifest)
	c.field = fs
	if err != nil {
		return err
	}

	marshalledFs := []byte(fs.Marshal())
	fsDigest := fmt.Sprintf("%x", sha256.Sum256(marshalledFs))

	// if new fs, write it to cache
	if !c.blobCache.Check(fsDigest) {
		fsWriter, err := c.blobCache.Add(fsDigest)
		if err != nil {
			return err
		}
		_, err = fsWriter.Write(marshalledFs)
		if err != nil {
			c.blobCache.Del(fsDigest)
			return err
		}
		defer fsWriter.Close()
	}

	c.updateImageInfo(targetUrl)

	for i, manifest := range c.manifests {
		// update manifest with new layer
		c.manifests[i].Layers = append(manifest.Layers, v1.Descriptor{
			MediaType: TwoDfsMediaType,
			Digest:    digest.Digest(fmt.Sprintf("sha256:%s", fsDigest)),
			Size:      int64(len(marshalledFs)),
		})
		if c.manifests[i].Annotations != nil {
			c.manifests[i].Annotations["org.opencontainers.image.url"] = fmt.Sprintf("https://%s/%s", c.registry, c.repository)
			c.manifests[i].Annotations["org.opencontainers.image.version"] = c.tag
		}
		if c.index.Manifests[i].Annotations != nil {
			c.index.Manifests[i].Annotations["org.opencontainers.image.url"] = fmt.Sprintf("https://%s/%s", c.registry, c.repository)
			c.index.Manifests[i].Annotations["org.opencontainers.image.version"] = c.tag
		}
	}

	// re-compute manifest digests and update index and caches
	for i, _ := range c.index.Manifests {
		marshalledManifest, err := json.Marshal(c.manifests[i])
		if err != nil {
			return err
		}
		manifestDigest := fmt.Sprintf("%x", sha256.Sum256(marshalledManifest))
		// update manifest cache
		if !c.blobCache.Check(manifestDigest) {
			manifestWriter, err := c.blobCache.Add(manifestDigest)
			if err != nil {
				return err
			}
			_, err = manifestWriter.Write(marshalledManifest)
			if err != nil {
				manifestWriter.Close()
				return err
			}
			manifestWriter.Close()
		}
		c.index.Manifests[i].Digest = digest.Digest(fmt.Sprintf("sha256:%s", manifestDigest))
	}

	// update index cache
	indexBytes, err := json.Marshal(c.index)
	if err != nil {
		return err
	}

	c.indexCache.Del(fmt.Sprintf("%x", sha256.Sum256([]byte(c.url))))
	indexWriter, err := c.indexCache.Add(fmt.Sprintf("%x", sha256.Sum256([]byte(c.url))))
	if err != nil {
		return err
	}
	_, err = indexWriter.Write(indexBytes)
	if err != nil {
		return err
	}
	return nil
}

func (c *containerImage) downloadManifests() error {
	for _, manifest := range c.index.Manifests {

		if manifest.Digest.Algorithm() != digest.SHA256 {
			return fmt.Errorf("unsupported digest algorithm: %s", manifest.Digest.Algorithm().String())
		}

		// check if blob cached
		manifestCached := c.blobCache.Check(manifest.Digest.Encoded())
		if manifestCached {
			// blob already cached, continue
			log.Printf("%s [CACHED]", manifest.Digest.Encoded())
		} else {
			// download blob
			c.downloadAndCache(manifest.Digest, v1.MediaTypeImageManifest)
		}

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
	configCached := c.blobCache.Check(manifest.Config.Digest.Encoded())
	if !configCached {
		// blob not cached cached, downloading
		c.downloadAndCache(manifest.Config.Digest, manifest.Config.MediaType)

	} else {
		log.Printf("%s [CACHED]", manifest.Config.Digest.Encoded())
	}

	// download layers
	for _, layer := range manifest.Layers {
		if layer.Digest.Algorithm() != digest.SHA256 {
			return fmt.Errorf("unsupported digest algorithm: %s", layer.Digest.Algorithm().String())
		}
		cached := c.blobCache.Check(layer.Digest.Encoded())
		if !cached {
			// blob not cached cached, downloading
			// TODO: this can be parallelized!
			err := c.downloadAndCache(layer.Digest, layer.MediaType)
			if err != nil {
				log.Printf("Error downloading layer: %v", err)
				return err
			}
		} else {
			log.Printf("%s [CACHED]", layer.Digest.Encoded())
		}
	}
	return nil
}

func (c *containerImage) downloadAndCache(downloadDigest digest.Digest, mediaType string) error {
	if downloadDigest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("unsupported digest algorithm: %s", downloadDigest.Algorithm().String())
	}

	log.Printf("%s [DOWNLOADING]", downloadDigest.Encoded())

	var readCloser io.ReadCloser
	var err error
	if mediaType == v1.MediaTypeImageManifest {
		readCloser, err = DownloadManifest(
			OciImageLink{
				Registry:   c.registry,
				Repository: c.repository,
				Tag:        c.tag,
			},
			downloadDigest.String(),
		)
		if err != nil {
			return err
		}
	} else {
		ctx := context.Background()
		readCloser, err = DownloadBlob(
			ctx,
			OciImageLink{
				Registry:   c.registry,
				Repository: c.repository,
				Tag:        c.tag,
			},
			downloadDigest,
			mediaType,
		)
		if err != nil {
			return err
		}
		defer readCloser.Close()
	}

	// upload blob to cache store
	uploadWriter, err := c.blobCache.Add(downloadDigest.Encoded())
	if err != nil {
		return err
	}
	_, err = io.Copy(uploadWriter, readCloser)
	if err != nil {
		c.blobCache.Del(downloadDigest.Encoded())
		return err
	}
	defer uploadWriter.Close()

	if !c.blobCache.Check(downloadDigest.Encoded()) {
		c.blobCache.Del(downloadDigest.Encoded())
		return fmt.Errorf("blob integrity check failed, please retry")
	}

	return nil
}

func (c *containerImage) GetExporter() FieldExporter {
	return c
}

func (c *containerImage) buildFiled(manifest filesystem.TwoDFsManifest) (filesystem.Field, error) {

	tmpFolder := filepath.Join(os.TempDir(), fmt.Sprintf("%x-field", sha256.Sum256([]byte(c.url))))
	if _, err := os.Stat(tmpFolder); err == nil {
		os.RemoveAll(tmpFolder)
	}
	os.Mkdir(tmpFolder, 0755)
	defer os.RemoveAll(tmpFolder)

	//empty Field
	f := filesystem.GetField()

	for _, a := range manifest.Allotments {
		//create allotment folder
		allotmentFolder := filepath.Join(tmpFolder, fmt.Sprintf("r%d-c%d", a.Row, a.Col))
		os.Mkdir(allotmentFolder, 0755)

		//create destination file
		dstPath := a.Dst
		//check if first characted or dstPath is a slash
		if dstPath[0] == '/' {
			dstPath = dstPath[1:]
		}
		dst, err := createFileWithDirs(filepath.Join(allotmentFolder, dstPath))
		if err != nil {
			return nil, err
		}
		fmt.Printf("File %s [COPY] \n", dst.Name())

		//open source file
		src, err := os.Open(a.Src)
		if err != nil {
			dst.Close()
			return nil, err
		}

		//write allotment content
		_, err = io.Copy(dst, src)
		dst.Close()
		src.Close()
		if err != nil {
			return nil, err
		}

		//compress allotment folder
		archiveName, err := compress.CompressFolder(allotmentFolder)
		if err != nil {
			return nil, err
		}
		archive, err := os.Open(archiveName)
		if err != nil {
			return nil, err
		}

		//cache blob
		archive.Seek(0, 0)
		digest := compress.CalculateSha256Digest(archive)
		if !c.blobCache.Check(digest) {
			blobWriter, err := c.blobCache.Add(digest)
			if err != nil {
				archive.Close()
				return nil, err
			}
			archive.Seek(0, 0)
			_, err = io.Copy(blobWriter, archive)
			blobWriter.Close()
			archive.Close()
			if err != nil {
				c.blobCache.Del(digest)
				return nil, err
			}
			fmt.Printf("Alltoment %d/%d %s [CREATED] \n", a.Row, a.Col, digest)
		}

		//add allotments

		f.AddAllotment(filesystem.Allotment{
			Row:      a.Row,
			Col:      a.Col,
			Digest:   digest,
			FileName: dstPath,
		})
	}

	return f, nil
}

func createFileWithDirs(p string) (*os.File, error) {
	// Extract the directory path from the full path
	dir := filepath.Dir(p)

	// Create all necessary directories using MkdirAll
	err := os.MkdirAll(dir, 0755) // Change 0755 to desired permission mode
	if err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Create the file using os.Create
	f, err := os.Create(p)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	return f, nil
}
