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
	"strconv"
	"strings"
	"sync"

	"log"

	"github.com/2DFS/2dfs-builder/cache"
	compress "github.com/2DFS/2dfs-builder/compress"
	"github.com/2DFS/2dfs-builder/filesystem"
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
	// KeyStoreContextKey is the context key for the blob store
	KeyStoreContextKey contextKeyType = "keyStore"
	// 2dfs media type
	TwoDfsMediaType = "application/vnd.oci.image.layer.v1.2dfs.field"
	// image name annotation
	ImageNameAnnotation = "2dfs.image.name"
	//semantic tag partition init char
	partitionInit = `--`
	//semantic tag partition split char
	partitionSplitChar = `.`
	//semantic partition regex patter
	semanticTagPattern = partitionInit + `\d+\` + partitionSplitChar + `\d+\` + partitionSplitChar + `\d+\` + partitionSplitChar + `\d+`
)

var PullPushProtocol = "https"

type containerImage struct {
	index          v1.Index
	indexHash      string
	registry       string
	repository     string
	tag            string
	url            string
	platforms      []string
	partitions     []partition
	partitionTag   string
	indexCache     cache.CacheStore
	blobCache      cache.CacheStore
	keyDigestCache cache.CacheStore
	field          filesystem.Field
	manifests      []v1.Manifest
	configs        []v1.Image
	cacheLock      sync.Mutex
	stargzOptions  StargzOptions
}

type StargzOptions struct {
	Enabled   bool
	ChunkSize int
}

type CacheKeys struct {
	Keys []FileCacheKey `json:"keys"`
}

type FileCacheKey struct {
	Destination      string `json:"destination"`
	DiffID           string `json:"diffID"`
	CompressedSha    string `json:"compressedSha"`
	TOCDigest        string `json:"tocDigest,omitempty"`
	UncompressedSize int64  `json:"uncompressedSize,omitempty"`
}

type partition struct {
	x1 int
	y1 int
	x2 int
	y2 int
}

type Image interface {
	AddField(manifest filesystem.TwoDFsManifest, targetImage string) error
	GetIndex() []byte
	GetExporter(args ...string) (FieldExporter, error)
}

func NewImage(ctx context.Context, url string, forcepull bool, platforms []string) (Image, error) {
	return NewImageWithStargzOptions(ctx, url, forcepull, platforms, StargzOptions{})
}

func NewImageWithStargzOptions(ctx context.Context, url string, forcepull bool, platforms []string, stargzOptions StargzOptions) (Image, error) {

	ctxIndexPosition := ctx.Value(IndexStoreContextKey)
	indexStoreLocation := ""
	if ctxIndexPosition != nil {
		indexStoreLocation = ctxIndexPosition.(string)
	} else {
		return nil, fmt.Errorf("index store location not found in context")
	}

	ctxBlobPosition := ctx.Value(BlobStoreContextKey)
	blobStoreLocation := ""
	if ctxBlobPosition != nil {
		blobStoreLocation = ctxBlobPosition.(string)
	} else {
		return nil, fmt.Errorf("blob store location not found in context")
	}

	ctxKeyPosition := ctx.Value(KeyStoreContextKey)
	keyStoreLocation := ""
	if ctxKeyPosition != nil {
		keyStoreLocation = ctxKeyPosition.(string)
	} else {
		return nil, fmt.Errorf("key store location not found in context")
	}

	imgstore, err := cache.NewCacheStore(indexStoreLocation)
	if err != nil {
		return nil, err
	}
	blobstore, err := cache.NewCacheStore(blobStoreLocation)
	if err != nil {
		return nil, err
	}
	blobdigeststore, err := cache.NewCacheStore(keyStoreLocation)
	if err != nil {
		return nil, err
	}

	img := &containerImage{
		indexCache:     imgstore,
		blobCache:      blobstore,
		keyDigestCache: blobdigeststore,
		manifests:      []v1.Manifest{},
		configs:        []v1.Image{},
		platforms:      platforms,
		cacheLock:      sync.Mutex{},
		stargzOptions:  stargzOptions,
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

func GetLocalImage(ctx context.Context, reference string) (Image, error) {

	ctxIndexPosition := ctx.Value(IndexStoreContextKey)
	indexStoreLocation := ""
	if ctxIndexPosition != nil {
		indexStoreLocation = ctxIndexPosition.(string)
	} else {
		return nil, fmt.Errorf("index store location not found in context")
	}

	ctxBlobPosition := ctx.Value(BlobStoreContextKey)
	blobStoreLocation := ""
	if ctxBlobPosition != nil {
		blobStoreLocation = ctxBlobPosition.(string)
	} else {
		return nil, fmt.Errorf("blob store location not found in context")
	}

	ctxKeyPosition := ctx.Value(KeyStoreContextKey)
	keyStoreLocation := ""
	if ctxKeyPosition != nil {
		keyStoreLocation = ctxKeyPosition.(string)
	} else {
		return nil, fmt.Errorf("key store location not found in context")
	}

	imgstore, err := cache.NewCacheStore(indexStoreLocation)
	if err != nil {
		return nil, err
	}
	blobstore, err := cache.NewCacheStore(blobStoreLocation)
	if err != nil {
		return nil, err
	}
	blobdigeststore, err := cache.NewCacheStore(keyStoreLocation)
	if err != nil {
		return nil, err
	}

	img := &containerImage{
		indexCache:     imgstore,
		blobCache:      blobstore,
		keyDigestCache: blobdigeststore,
		manifests:      []v1.Manifest{},
		cacheLock:      sync.Mutex{},
	}

	idxReader, err := imgstore.Get(reference)
	if err != nil {
		// if reference not found, try getting the image using the url
		img.updateImageInfo(reference)
		fmt.Printf("Resolving image url %s locally...\n", img.indexHash)
		idxReader, err = imgstore.Get(img.indexHash)
		if err != nil {
			return nil, err
		}
	}
	defer idxReader.Close()

	idx, err := ReadIndex(idxReader)
	if err != nil {
		return nil, err
	}

	img.index = idx
	if img.url == "" {
		img.updateImageInfo(idx.Annotations[ImageNameAnnotation])
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

	// check if image requires partitioning
	if len(img.partitions) > 0 {
		fmt.Printf("Partitioning the image...\n")
		idx.Annotations[ImageNameAnnotation] = img.registry + "/" + img.repository + ":" + img.partitionTag
		img.indexHash = fmt.Sprintf("%x", sha256.Sum256([]byte(idx.Annotations[ImageNameAnnotation])))
		err = img.partition()
		if err != nil {
			return nil, err
		}
		//delete field for current image
		img.field = nil
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

	indexReader, err := c.indexCache.Get(c.indexHash)
	if err == nil {
		log.Default().Printf("%s [CACHED] \n", c.url)
		// load index from cache
		defer indexReader.Close()
		index, err := ReadIndex(indexReader)
		if err != nil {
			// if error reading index, remove it from cache
			log.Default().Printf("unable to read %s from cache, removing it... try again please \n", url)
			c.indexCache.Del(c.indexHash)
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
		Reference:  c.tag,
	})
	if err != nil {
		return err
	}

	if index.Annotations == nil {
		index.Annotations = make(map[string]string)
	}
	index.Annotations[ImageNameAnnotation] = c.url
	log.Default().Println("Index downloaded")

	index = c.filterByPlatform(index)

	// save index to cache
	uploadWriter, err := c.indexCache.Add(c.indexHash)
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
	c.partitions = []partition{}
	if len(urlParts) == 1 {
		c.registry = "docker.io"
		c.repository = url
	} else {
		registryRegex := regexp.MustCompile(`\b([a-z,\d,:]+)\.?\s*(?:\b([a-z]+)\.?\s*)*`)
		registry := urlParts[0]
		c.repository = fmt.Sprintf(urlParts[1])
		if registryRegex.FindStringIndex(registry) == nil {
			c.registry = "docker.io"
		} else {
			c.registry = registry
		}
	}

	// add default library repo if not present
	if strings.Count(c.repository, "/") == 0 {
		c.repository = "library/" + c.repository
	}

	//check tag and partition
	tagAndRepo := strings.Split(c.repository, ":")
	c.tag = "latest"
	if len(tagAndRepo) == 2 {
		c.tag = tagAndRepo[1]
		c.partitionTag = c.tag //default partition tag is the tag itself, even without partitions
		c.repository = tagAndRepo[0]
		re := regexp.MustCompile(semanticTagPattern)
		matches := re.FindAllString(c.tag, -1)
		if len(matches) > 0 {
			//semantic tag with partition
			fmt.Printf("Semantic tag with partition detected %s\n", c.tag)
			c.tag = strings.Split(c.tag, partitionInit)[0]
			for _, p := range matches {
				part, err := parsePartition(strings.Replace(p, partitionInit, "", -1))
				if err != nil {
					fmt.Printf("[WARNING] Invalid partition %s, skipping...\n", p)
					continue
				}
				c.partitions = append(c.partitions, part)
			}
		}
	}
	c.url = c.registry + "/" + c.repository + ":" + c.tag
	c.indexHash = fmt.Sprintf("%x", sha256.Sum256([]byte(c.url)))
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
		fmt.Printf("Field %s [CREATED]\n", fsDigest)
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
	} else {
		fmt.Printf("Field %s [CACHED]\n", fsDigest)
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
	c.index.Annotations[ImageNameAnnotation] = c.url

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
		} else {
			fmt.Printf("%s [CACHED]\n", manifestDigest)
		}
		c.index.Manifests[i].Digest = digest.Digest(fmt.Sprintf("sha256:%s", manifestDigest))
		c.index.Manifests[i].Size, err = c.blobCache.GetSize(manifestDigest)
		if err != nil {
			return err
		}
	}

	// update index cache
	indexBytes, err := json.Marshal(c.index)
	if err != nil {
		return err
	}

	c.indexCache.Del(c.indexHash)
	indexWriter, err := c.indexCache.Add(c.indexHash)
	if err != nil {
		return err
	}
	_, err = indexWriter.Write(indexBytes)
	if err != nil {
		return err
	}
	return nil
}

func (c *containerImage) GetIndex() []byte {
	index, err := json.Marshal(c.index)
	if err != nil {
		log.Printf("Error marshalling index: %v", err)
		return nil
	}
	return index
}

func (c *containerImage) partition() error {

	partitionAllotment := []filesystem.Allotment{}

	for i, manifest := range c.manifests {
		filteredLayers := []v1.Descriptor{}
		rootfsLayers := c.configs[i].RootFS
		//removing 2dfs temporary layer if present
		for _, layer := range manifest.Layers {
			if layer.MediaType == TwoDfsMediaType {
				//if 2dfs layer parse field and partition allotments
				c.readField(layer.Digest.Encoded())
				if len(partitionAllotment) == 0 {
					if c.field != nil {
						for allotment := range c.field.IterateAllotments() {
							//skip empty allotments
							if allotment.Digest == "" {
								continue
							}
							for _, p := range c.partitions {
								if allotment.Row >= p.x1 && allotment.Row <= p.x2 && allotment.Col >= p.y1 && allotment.Col <= p.y2 {
									partitionAllotment = append(partitionAllotment, allotment)
									//TODO remove duplicated
								}
							}
						}
					}
				}
			} else {
				filteredLayers = append(filteredLayers, layer)
			}
		}
		if len(partitionAllotment) > 0 {
			//adding partitioned layers
			for _, p := range partitionAllotment {
				blobSize, err := c.blobCache.GetSize(p.Digest)
				if err != nil {
					return err
				}
				fmt.Printf("Partition %s [CREATING]\n", p.Digest)
				filteredLayers = append(filteredLayers, v1.Descriptor{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    digest.Digest(fmt.Sprintf("sha256:%s", p.Digest)),
					Size:      blobSize,
				})
				rootfsLayers.DiffIDs = append(rootfsLayers.DiffIDs, digest.Digest(fmt.Sprintf("sha256:%s", p.DiffID)))
			}
		} else {
			return fmt.Errorf("no 2DFS partitions found. Make sure the image has format OCI+2DFS and that the partition matches the allotments")
		}
		c.manifests[i].Layers = filteredLayers
		c.configs[i].RootFS = rootfsLayers

		marshalledConfig, _ := json.Marshal(c.configs[i])
		c.manifests[i].Config.Digest = digest.Digest(fmt.Sprintf("sha256:%x", sha256.Sum256(marshalledConfig)))
	}

	for i, _ := range c.index.Manifests {
		marshalledManifest, err := json.Marshal(c.manifests[i])
		if err != nil {
			return err
		}
		manifestDigest := fmt.Sprintf("%x", sha256.Sum256(marshalledManifest))
		configDigest := c.manifests[i].Config.Digest.Encoded()
		// update manifest
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
		} else {
			fmt.Printf("%s [CACHED]\n", manifestDigest)
		}
		// update config
		if !c.blobCache.Check(configDigest) {
			configWriter, err := c.blobCache.Add(configDigest)
			if err != nil {
				return err
			}
			marshalledConfig, _ := json.Marshal(c.configs[i])
			_, err = configWriter.Write(marshalledConfig)
			if err != nil {
				configWriter.Close()
				return err
			}
			configWriter.Close()
		} else {
			fmt.Printf("%s [CACHED]\n", configDigest)
		}

		c.index.Manifests[i].Digest = digest.Digest(fmt.Sprintf("sha256:%s", manifestDigest))
		c.index.Manifests[i].Size, err = c.blobCache.GetSize(manifestDigest)
		if err != nil {
			return err
		}
	}

	// update index cache
	indexBytes, err := json.Marshal(c.index)
	if err != nil {
		return err
	}

	c.indexCache.Del(c.indexHash)
	indexWriter, err := c.indexCache.Add(c.indexHash)
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
		manifest, _, _, err := ReadManifest(manifestCachereader)
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

	// Load config from cache
	configCachereader, err := c.blobCache.Get(manifest.Config.Digest.Encoded())
	if err != nil {
		return err
	}
	defer configCachereader.Close()
	config, err := ReadConfig(configCachereader)
	if err != nil {
		return err
	}
	c.configs = append(c.configs, config)

	// download layers
	success := make(chan bool, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		go func() {
			if layer.Digest.Algorithm() != digest.SHA256 {
				log.Printf("ERROR: unsupported digest algorithm: %s", layer.Digest.Algorithm().String())
				success <- false
				return
			}
			cached := c.blobCache.Check(layer.Digest.Encoded())
			if !cached {
				// blob not cached cached, downloading
				// TODO: this can be parallelized!
				err := c.downloadAndCache(layer.Digest, layer.MediaType)
				if err != nil {
					log.Printf("Error downloading layer: %v", err)
					success <- false
					return
				}
			} else {
				log.Printf("%s [CACHED]", layer.Digest.Encoded())
			}
			success <- true
		}()
	}
	terminate := false
	for i := 0; i < len(manifest.Layers); i++ {
		if !<-success {
			terminate = true
		}
	}
	if terminate {
		return fmt.Errorf("error during blob download")
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
				Reference:  c.tag,
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
				Reference:  c.tag,
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
	copyBuffer := make([]byte, 1024*1024)
	_, err = io.CopyBuffer(uploadWriter, readCloser, copyBuffer)
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

func (c *containerImage) GetExporter(args ...string) (FieldExporter, error) {
	if len(args) == 2 {
		os := args[0]
		arch := args[1]
		if os == "" || arch == "" {
			return c, nil
		}

		selectedManifestIdx := -1
		for i, manifest := range c.index.Manifests {
			if manifest.Platform.OS == os && manifest.Platform.Architecture == arch {
				selectedManifestIdx = i
				break
			}
		}
		if selectedManifestIdx == -1 {
			return nil, fmt.Errorf("manifest not found for %s/%s", os, arch)
		} else {
			c.index.Manifests = []v1.Descriptor{c.index.Manifests[selectedManifestIdx]}
			c.manifests = []v1.Manifest{c.manifests[selectedManifestIdx]}
			fmt.Printf("Selected manifest %s/%s\n", os, arch)
		}
	}
	return c, nil
}

func (c *containerImage) buildFiled(manifest filesystem.TwoDFsManifest) (filesystem.Field, error) {

	tmpFolder := filepath.Join(os.TempDir(), fmt.Sprintf("%x-field", c.indexHash))
	if _, err := os.Stat(tmpFolder); err == nil {
		os.RemoveAll(tmpFolder)
	}
	os.Mkdir(tmpFolder, 0755)
	defer os.RemoveAll(tmpFolder)

	//pupulate field with allotments
	f := filesystem.GetField()

	success := make(chan bool, len(manifest.Allotments))
	for _, a := range manifest.Allotments {
		go func() {
			err := c.buildAllotment(a, f)
			if err != nil {
				success <- false
				log.Default().Printf("ERROR: %v\n", err)
			} else {
				success <- true
			}
		}()
	}
	terminate := false
	for i := 0; i < len(manifest.Allotments); i++ {
		if !<-success {
			terminate = true
		}
	}
	if terminate {
		return nil, fmt.Errorf("error during allotment build procedure")
	}

	return f, nil
}

// builds allotment tar, gzips it and stores in./2dfs/blobs.
// The resulting allotment with its digest and position is added to the provided field
func (c *containerImage) buildAllotment(a filesystem.AllotmentManifest, f filesystem.Field) error {

	var tocDigest string
	var uncompressedSize int64
	// digest of all files in src of an allotment
	fileSha, err := compress.CalculateMultiSha256Digest(a.Src.List)
	if err != nil {
		return err
	}

	compressedSha, diffID := func() (string, string) {
		c.cacheLock.Lock()
		defer c.cacheLock.Unlock()
		// check uncompressed-keys cache if an entry with fileSha exists
		keyDigestReader, err := c.keyDigestCache.Get(fileSha)
		// check if item is cached
		if err == nil {
			defer keyDigestReader.Close()
			cacheKeys, err := ParseCacheKey(keyDigestReader)
			if err != nil {
				log.Fatal(err)
			}
			cached, err := GetFileSha(cacheKeys, a.Dst.List)
			if err == nil {
				log.Printf("File %s [CACHED] \n", a.Src)
				tocDigest = cached.TOCDigest
				uncompressedSize = cached.UncompressedSize
				return cached.CompressedSha, cached.DiffID
			}

			log.Printf("%v", err)
			log.Printf("File %s no cache entry found \n", a.Src)
		}
		return "", ""
	}()

	// if no cache entry found, generate one
	if compressedSha == "" {
		log.Printf("File %s [COPY] \n", a.Src)

		tarPath, err := compress.TarFile(a.Src.List, a.Dst.List)
		if err != nil {
			return err
		}
		tarReader, err := os.Open(tarPath)
		if err != nil {
			return err
		}
		defer tarReader.Close()
		defer os.Remove(tarPath)

		log.Printf("File %s [COMPRESSING] \n", a.Src)

		if c.stargzOptions.Enabled {
			log.Printf("use stargz compression\n")
			// Use stargz compression
			stargzResult, err := compress.TarToStargz(tarPath, c.stargzOptions.ChunkSize)
			if err != nil {
				return err
			}
			defer stargzResult.CompressedBlob.Close()

			// Write blob to a temp file while computing the compressed digest
			tmpFile, err := os.CreateTemp("", "stargz-blob-*")
			if err != nil {
				return err
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			compressedSha, _, err = compress.CalculateStargzDigest(io.TeeReader(stargzResult.CompressedBlob, tmpFile))
			tmpFile.Close()
			if err != nil {
				return err
			}

			// Get DiffID from the stargz blob (must be called after blob is fully read)
			// This is the correct DiffID that matches what the stargz snapshotter will calculate
			// when decompressing the layer, as estargz adds TOC to the archive
			diffID = stargzResult.CompressedBlob.DiffID().Encoded()
			log.Printf("diff id %s\n", diffID)

			tocDigest = stargzResult.TOCDigest.String()
			log.Printf("tocDigest value: '%s'\n", tocDigest)
			// For uncompressed size, we'll use the original tar size for now
			tarStat, err := os.Stat(tarPath)
			if err != nil {
				return err
			}
			uncompressedSize = tarStat.Size()

			//add stargz allotment cache reference
			c.cacheLock.Lock()
			c.upsertCacheKey(fileSha, FileCacheKey{
				DiffID:           diffID,
				CompressedSha:    compressedSha,
				TOCDigest:        tocDigest,
				UncompressedSize: uncompressedSize,
			}, a.Dst.List)
			c.cacheLock.Unlock()

			if !c.blobCache.Check(compressedSha) {
				blobWriter, err := c.blobCache.Add(compressedSha)
				if err != nil {
					return err
				}
				tmpReader, err := os.Open(tmpPath)
				if err != nil {
					blobWriter.Close()
					return err
				}
				copyBuffer := make([]byte, 1024*1024)
				_, err = io.CopyBuffer(blobWriter, tmpReader, copyBuffer)
				tmpReader.Close()
				blobWriter.Close()
				if err != nil {
					c.blobCache.Del(compressedSha)
					return err
				}
				log.Printf("Stargz Allotment %d/%d %s [CREATED] \n", a.Row, a.Col, compressedSha)
			}
		} else {
			// For standard gzip, calculate DiffID from the original tar
			// (unlike stargz, gzip decompression produces the exact same tar)
			diffID = compress.CalculateSha256Digest(tarReader)
			tarReader.Seek(0, 0)

			// Use standard gzip compression
			archiveName, err := compress.TarToGz(tarPath)
			if err != nil {
				return err
			}
			archive, err := os.Open(archiveName)
			if err != nil {
				return err
			}
			defer archive.Close()
			defer os.Remove(archiveName)
			compressedSha = compress.CalculateSha256Digest(archive)

			//add uncompressed allotment cache reference
			c.cacheLock.Lock()
			c.upsertCacheKey(fileSha, FileCacheKey{
				DiffID:        diffID,
				CompressedSha: compressedSha,
			}, a.Dst.List)
			c.cacheLock.Unlock()

			if !c.blobCache.Check(compressedSha) {
				blobWriter, err := c.blobCache.Add(compressedSha)
				if err != nil {
					return err
				}
				archive.Seek(0, 0)
				copyBuffer := make([]byte, 1024*1024)
				_, err = io.CopyBuffer(blobWriter, archive, copyBuffer)
				blobWriter.Close()
				archive.Close()
				if err != nil {
					c.blobCache.Del(compressedSha)
					return err
				}
				log.Printf("Alltoment %d/%d %s [CREATED] \n", a.Row, a.Col, compressedSha)
			}
		}
	}

	allotment := filesystem.Allotment{
		Row:    a.Row,
		Col:    a.Col,
		Digest: compressedSha,
		DiffID: diffID,
	}

	if c.stargzOptions.Enabled {
		allotment.TOCDigest = tocDigest
		allotment.UncompressedSize = uncompressedSize
		allotment.IsStargz = true
	}

	f.AddAllotment(allotment)

	return nil
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

func parsePartition(p string) (partition, error) {
	parts := strings.Split(p, partitionSplitChar)
	result := partition{}
	if len(parts) != 4 {
		return result, fmt.Errorf("invalid partition %s", p)
	}
	var err error
	result.x1, err = strconv.Atoi(parts[0])
	if err != nil {
		return result, err
	}
	result.y1, err = strconv.Atoi(parts[1])
	if err != nil {
		return result, err
	}
	result.x2, err = strconv.Atoi(parts[2])
	if err != nil {
		return result, err
	}
	result.y2, err = strconv.Atoi(parts[3])
	if err != nil {
		return result, err
	}
	return result, nil
}

func (c *containerImage) readField(fieldHash string) error {
	if c.field == nil {
		fieldReader, err := c.blobCache.Get(fieldHash)
		if err != nil {
			return err
		}
		defer fieldReader.Close()
		fullField, err := io.ReadAll(fieldReader)
		if err != nil {
			return err
		}
		field, err := filesystem.GetField().Unmarshal(string(fullField[:]))
		if err != nil {
			return err
		}
		c.field = field
	}
	return nil
}

func (c *containerImage) filterByPlatform(index v1.Index) v1.Index {
	//filtering out unwanted platforms based on user filter
	if len(c.platforms) > 0 {
		manifestList := []v1.Descriptor{}
		for _, manifest := range index.Manifests {
			for _, plat := range c.platforms {
				if fmt.Sprintf("%s/%s", manifest.Platform.OS, manifest.Platform.Architecture) == plat {
					manifestList = append(manifestList, manifest)
				}
			}
		}
		index.Manifests = manifestList
	}
	return index
}

func (c *containerImage) upsertCacheKey(fileSha string, cacheFile FileCacheKey, dst []string) error {
	//convert destination to string
	destinationStr := strings.Join(dst[:], ",")
	cacheFile.Destination = destinationStr

	keyDigestReader, err := c.keyDigestCache.Get(fileSha)
	cachekey := CacheKeys{
		Keys: []FileCacheKey{},
	}
	// if cache entry found, read it so we can append the new key
	if err == nil {
		cachekey, err = ParseCacheKey(keyDigestReader)
		if err != nil {
			keyDigestReader.Close()
			return err
		}
		keyDigestReader.Close()
		c.keyDigestCache.Del(fileSha)
	}

	cachekey.Keys = append(cachekey.Keys, cacheFile)
	cachewriter, err := c.keyDigestCache.Add(fileSha)
	if err != nil {
		return err
	}
	defer cachewriter.Close()
	cacheKeyBytes, err := json.Marshal(cachekey)
	if err != nil {
		return err
	}
	_, err = cachewriter.Write(cacheKeyBytes)
	if err != nil {
		return err
	}

	return nil
}

func ParseCacheKey(reader io.Reader) (CacheKeys, error) {
	var cacheKey CacheKeys
	cacheKeyBytes, err := io.ReadAll(reader)
	if err != nil {
		return cacheKey, err
	}
	err = json.Unmarshal(cacheKeyBytes, &cacheKey)
	if err != nil {
		return cacheKey, err
	}
	return cacheKey, nil
}

// Given the file destination, and the CacheKeys, looks if any of the keys match the destination and returns the key and the sha of the file. Error otherwise.
func GetFileSha(keys CacheKeys, dst []string) (FileCacheKey, error) {
	destinationStr := strings.Join(dst, ",")
	for _, key := range keys.Keys {
		if key.Destination == destinationStr {
			return key, nil
		}
	}
	return FileCacheKey{}, fmt.Errorf("file not found in cache")
}
