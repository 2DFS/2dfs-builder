package oci

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/giobart/2dfs-builder/compress"
	"github.com/giobart/2dfs-builder/filesystem"
)

type FieldExporter interface {
	ExportAsTar(dst string) error
	Upload(OciImageLink) error
}

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
