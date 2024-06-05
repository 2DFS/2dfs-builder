package oci

import (
	"io"
	"os"
	"path/filepath"

	"github.com/giobart/2dfs-builder/compress"
)

type FieldExporter interface {
	ExportAsTar(dst string) error
	Upload(OciImageLink) error
}

func (image *containerImage) ExportAsTar(path string) error {

	// create a temporary folder
	tmpFolder := filepath.Join(os.TempDir(), image.repository, image.tag)
	if _, err := os.Stat(tmpFolder); err == nil {
		os.RemoveAll(tmpFolder)
	}
	os.Mkdir(tmpFolder, 0755)
	defer os.RemoveAll(tmpFolder)

	// copy index and blobs
	indexPath := filepath.Join(tmpFolder, "index.json")
	index, err := image.indexCache.Get(image.url)
	if err != nil {
		return err
	}
	defer index.Close()
	err = copyFile(index, indexPath)
	if err != nil {
		return err
	}

	// copy manifest, config and layers
	for i, manifest := range image.index.Manifests {
		// copy manifest
		manifestDigest := manifest.Digest.Encoded()
		manifestPath := filepath.Join(tmpFolder, "blobs", "sha256", manifestDigest)
		manifestReader, err := image.blobCache.Get(manifestDigest)
		if err != nil {
			return err
		}
		err = copyFile(manifestReader, manifestPath)
		if err != nil {
			manifestReader.Close()
			return err
		}
		manifestReader.Close()

		//copy config
		configDigest := image.manifests[i].Config.Digest.Encoded()
		configPath := filepath.Join(tmpFolder, "blobs", "sha256", configDigest)
		configReader, err := image.blobCache.Get(configDigest)
		if err != nil {
			return err
		}
		err = copyFile(configReader, configPath)
		if err != nil {
			configReader.Close()
			return err
		}
		configReader.Close()

		//copy layers
		for _, layer := range image.manifests[i].Layers {
			layerDigest := layer.Digest.Encoded()
			layerPath := filepath.Join(tmpFolder, "blobs", "sha256", layerDigest)
			layerReader, err := image.blobCache.Get(configDigest)
			if err != nil {
				return err
			}
			err = copyFile(layerReader, layerPath)
			if err != nil {
				layerReader.Close()
				return err
			}
			layerReader.Close()

		}
	}

	archive, err := compress.CompressFolder(tmpFolder)
	if err != nil {
		return err
	}
	archivereader, err := os.Open(archive)
	if err != nil {
		return err
	}

	return copyFile(archivereader, path)
}

func (e *containerImage) Upload(link OciImageLink) error {
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
