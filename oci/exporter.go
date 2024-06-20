package oci

import (
	"crypto/sha256"
	"fmt"
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
	tmpFolder := filepath.Join(os.TempDir(), fmt.Sprintf("%x", sha256.Sum256([]byte(image.url))))
	if _, err := os.Stat(tmpFolder); err == nil {
		os.RemoveAll(tmpFolder)
	}
	os.Mkdir(tmpFolder, 0755)
	defer os.RemoveAll(tmpFolder)

	// copy index and blobs
	indexPath := filepath.Join(tmpFolder, "index.json")
	index, err := image.indexCache.Get(fmt.Sprintf("%x", sha256.Sum256([]byte(image.url))))
	if err != nil {
		return err
	}
	defer index.Close()
	err = copyFile(index, indexPath)
	if err != nil {
		return err
	}
	fmt.Println("Index copied")

	// TODO: create oci layout file

	shaFolder := filepath.Join(tmpFolder, "blobs", "sha256")
	os.MkdirAll(shaFolder, os.ModePerm)

	// copy manifest, config and layers
	for i, manifest := range image.index.Manifests {
		// copy manifest
		manifestDigest := manifest.Digest.Encoded()
		manifestPath := filepath.Join(shaFolder, manifestDigest)
		err = image.exportBlobByDigest(manifestPath, manifestDigest)
		if err != nil {
			return err
		}
		fmt.Printf("%s [EXPORTED]\n", manifestDigest)

		//copy config
		configDigest := image.manifests[i].Config.Digest.Encoded()
		configPath := filepath.Join(tmpFolder, "blobs", "sha256", configDigest)
		err = image.exportBlobByDigest(configPath, configDigest)
		if err != nil {
			return err
		}
		fmt.Printf("%s [EXPORTED]\n", configDigest)

		//copy layers
		for _, layer := range image.manifests[i].Layers {
			layerDigest := layer.Digest.Encoded()
			layerPath := filepath.Join(tmpFolder, "blobs", "sha256", layerDigest)
			err = image.exportBlobByDigest(layerPath, layerDigest)
			if err != nil {
				return err
			}
			fmt.Printf("%s [EXPORTED]\n", layerDigest)
		}

	}

	//copy 2dfs
	for allotment := range image.field.IterateAllotments() {
		allotmentDigest := allotment.Digest
		allotmentPath := filepath.Join(tmpFolder, "blobs", "sha256", allotmentDigest)
		err = image.exportBlobByDigest(allotmentPath, allotmentDigest)
		if err != nil {
			return err
		}
		fmt.Printf("Field %d/%d [EXPORTED]\n", allotment.Row, allotment.Col)
	}

	// compress the folder
	fmt.Printf("%s [COMPRESSING...]\n", fmt.Sprintf("%x", sha256.Sum256([]byte(image.url))))
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
