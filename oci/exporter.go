package oci

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/giobart/2dfs-builder/compress"
	"github.com/giobart/2dfs-builder/filesystem"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type FieldExporter interface {
	ExportAsTar(dst string) error
	Upload() error
}

var uploadToken string = ""

func (image *containerImage) ExportAsTar(path string) error {

	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Start()

	// create a temporary folder
	s.Suffix = fmt.Sprintf("%s [EXPORTING...]\n", image.url)
	s.Restart()
	tmpFolder := filepath.Join(os.TempDir(), image.indexHash)
	if _, err := os.Stat(tmpFolder); err == nil {
		os.RemoveAll(tmpFolder)
	}
	os.Mkdir(tmpFolder, 0755)
	defer os.RemoveAll(tmpFolder)

	// copy index and blobs
	indexBytes, err := json.Marshal(image.index)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(tmpFolder, "index.json"), indexBytes, 0644)
	if err != nil {
		return err
	}
	fmt.Println("Index copied")

	// TODO: create oci layout file

	shaFolder := filepath.Join(tmpFolder, "blobs", "sha256")
	os.MkdirAll(shaFolder, os.ModePerm)

	// copy manifest, config and layers
	tdfslayer := ""
	for i, manifest := range image.index.Manifests {
		// copy manifest
		manifestDigest := manifest.Digest.Encoded()
		manifestPath := filepath.Join(shaFolder, manifestDigest)
		err = image.exportBlobByDigest(manifestPath, manifestDigest)
		if err != nil {
			return err
		}
		s.Suffix = fmt.Sprintf("%s [EXPORTED]\n", manifestDigest)

		//copy config
		configDigest := image.manifests[i].Config.Digest.Encoded()
		configPath := filepath.Join(tmpFolder, "blobs", "sha256", configDigest)
		err = image.exportBlobByDigest(configPath, configDigest)
		if err != nil {
			return err
		}
		s.Suffix = fmt.Sprintf("%s [EXPORTED]\n", configDigest)

		//copy layers
		for _, layer := range image.manifests[i].Layers {
			layerDigest := layer.Digest.Encoded()
			layerPath := filepath.Join(tmpFolder, "blobs", "sha256", layerDigest)
			err = image.exportBlobByDigest(layerPath, layerDigest)
			if err != nil {
				return err
			}
			s.Suffix = fmt.Sprintf("%s [EXPORTED]\n", layerDigest)
			if layer.MediaType == TwoDfsMediaType {
				tdfslayer = layerDigest
			}
		}

	}

	//update field if present
	if image.field == nil {
		if tdfslayer != "" {
			fieldReader, err := image.blobCache.Get(tdfslayer)
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
			image.field = field
		}
	}

	//export 2dfs if present and no partitioning required
	if image.field != nil {
		for allotment := range image.field.IterateAllotments() {
			allotmentDigest := allotment.Digest
			allotmentPath := filepath.Join(tmpFolder, "blobs", "sha256", allotmentDigest)
			err = image.exportBlobByDigest(allotmentPath, allotmentDigest)
			if err != nil {
				return err
			}
			s.Suffix = fmt.Sprintf("Field %d/%d [EXPORTED]\n", allotment.Row, allotment.Col)
		}
	}

	//add oci layout version
	ociLayout := []byte(`{"imageLayoutVersion": "1.0.0"}`)
	ociLayoutPath := filepath.Join(tmpFolder, "oci-layout")
	err = os.WriteFile(ociLayoutPath, ociLayout, 0644)
	if err != nil {
		return err
	}

	// compress the folder
	s = spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" %s [COMPRESSING...]\n", image.indexHash)
	s.Start()
	archive, err := compress.CompressFolder(tmpFolder)
	if err != nil {
		return err
	}
	archivereader, err := os.Open(archive)
	if err != nil {
		return err
	}
	s.Stop()

	return copyFile(archivereader, path)
}

func (image *containerImage) Upload() error {

	imageLink := OciImageLink{
		Registry:   image.registry,
		Repository: image.repository,
		Reference:  image.partitionTag,
	}

	log.Default().Printf("Pushing %s/%s:%s \n", imageLink.Registry, imageLink.Repository, imageLink.Reference)

	//Upload Blobs
	err := image.uploadBlobs(imageLink)
	if err != nil {
		log.Default().Printf("[ERROR] Error uploading blobs: %s", err)
		return err
	}

	// Index Upload
	err = image.uploadIndex(imageLink)
	if err != nil {
		log.Default().Printf("[ERROR] Error uploading blobs: %s", err)
		return err
	}

	return nil
}

func (image *containerImage) exportBlobByDigest(blobPath string, digest string) error {
	blobReader, err := image.blobCache.Get(digest)
	if err != nil {
		return err
	}
	defer blobReader.Close()
	err = copyFile(blobReader, blobPath)
	if err != nil {
		return err
	}
	return nil
}

func copyFile(src io.ReadCloser, dst string) error {
	dstF, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstF.Close()

	io.Copy(dstF, src)
	return nil
}

// Upload the index to the registry and retunrs the upload token used
func (image *containerImage) uploadIndex(link OciImageLink) error {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)

	s.Suffix = fmt.Sprintf("%s/%s [Uploading...]\n", image.indexHash[:10], v1.MediaTypeImageIndex)
	s.Start()

	indexBytes, err := json.Marshal(image.index)
	if err != nil {
		return err
	}
	uploadToken, err = UploadManifest(link, indexBytes, v1.MediaTypeImageIndex, uploadToken)

	s.Stop()

	return err
}

func (e *containerImage) uploadBlobs(link OciImageLink) error {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Start()

	// Upload manifest layers
	for _, manifest := range e.manifests {
		for _, layer := range manifest.Layers {
			layerDigest := layer.Digest.Encoded()
			layerMediaType := layer.MediaType

			//spinner
			s.Suffix = fmt.Sprintf("%s/%s [Uploading...]\n", layerDigest[:10], layerMediaType)
			s.Restart()

			err := e.postByBlobDigest(link, layerMediaType, layerDigest, int(layer.Size))
			if err != nil {
				return err
			}
		}
	}

	// Upload manifests configs
	for _, manifest := range e.manifests {
		configDigest := manifest.Config.Digest.Encoded()
		layerMediaType := manifest.Config.MediaType

		//spinner
		s.Suffix = fmt.Sprintf("%s/%s [Uploading...]\n", configDigest[:10], layerMediaType)
		s.Restart()

		err := e.postByBlobDigest(link, v1.MediaTypeImageConfig, configDigest, int(manifest.Config.Size))
		if err != nil {
			return err
		}
	}

	// Upload manifests
	for _, manifest := range e.index.Manifests {
		manifestDigest := manifest.Digest.Encoded()
		layerMediaType := manifest.MediaType

		//spinner
		s.Suffix = fmt.Sprintf("%s/%s [Uploading...]\n", manifestDigest[:10], layerMediaType)
		s.Restart()

		err := e.postByBlobDigest(link, layerMediaType, manifestDigest, int(manifest.Size))
		if err != nil {
			return err
		}
	}

	s.Stop()
	return nil
}

func (image *containerImage) postByBlobDigest(link OciImageLink, mediaType string, digest string, size int) error {
	blobReader, err := image.blobCache.Get(digest)
	if err != nil {
		return err
	}
	defer blobReader.Close()

	// since this is not an iindex upload, we use blob digest as reference instead of a Tag
	linkWithReference := OciImageLink{
		Registry:   link.Registry,
		Repository: link.Repository,
		Reference:  digest,
	}

	switch mediaType {
	case v1.MediaTypeImageManifest:
		manifestbytes, err := io.ReadAll(blobReader)
		if err != nil {
			return err
		}
		uploadToken, err = UploadManifest(linkWithReference, manifestbytes, mediaType, uploadToken)
		if err != nil {
			return err
		}
	default:
		uploadToken, err = UploadBlob(linkWithReference, blobReader, size, uploadToken)
		if err != nil {
			return err
		}
	}
	return nil
}
