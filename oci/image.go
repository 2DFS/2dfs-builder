package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/giobart/2dfs-builder/blobstore"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// DefaultRegistry is the default registry to use
	DefaultRegistry = "index.docker.io"
	// IndexStoreContextKey is the context key for the index store
	IndexStoreContextKey = "indexStore"
)

type ContainerImage struct {
	path  string
	index v1.Index
}

func NewContainerImage(url string, ctx context.Context) (*ContainerImage, error) {
	index, err := LoadIndex(url, ctx)
	if err != nil {
		return nil, err
	}
	return &ContainerImage{
		index: index,
	}, nil
}

func LoadIndex(url string, ctx context.Context) (v1.Index, error) {
	// if path is an URL use Distribution spec to download image index
	// if path is a local file use fsutil.ReadFile

	ctxPosition := ctx.Value(IndexStoreContextKey)
	storeLocation := ""
	if ctxPosition != nil {
		storeLocation = ctxPosition.(string)
	} else {
		return v1.Index{}, fmt.Errorf("Index store location not found in context")
	}

	//check index local cache first
	imgstore, err := blobstore.NewBlobStore(storeLocation)
	if err != nil {
		return v1.Index{}, err
	}
	indexReader, err := imgstore.GetBlob(url)
	if err == nil {
		// load index from cache
		defer indexReader.Close()
		index, err := ReadIndex(indexReader)
		if err != nil {
			return v1.Index{}, err
		}
		return index, nil
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

	// save index to cache
	uploadWriter, err := imgstore.UploadBlob(url)
	if err != nil {
		return v1.Index{}, err
	}
	defer uploadWriter.Close()
	indexBytes, err := json.Marshal(index)
	if err != nil {
		return v1.Index{}, err
	}
	_, err = uploadWriter.Write(indexBytes)
	if err != nil {
		return v1.Index{}, err
	}

	return index, nil
}
