package oci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Start()

	// Start Index Upload
	s.Suffix = fmt.Sprintf("%s [Uploading...]\n", image.url)
	s.Restart()
	_, err := image.uploadIndex(OciImageLink{})
	if err != nil {
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
func (image *containerImage) uploadIndex(link OciImageLink) (string, error) {

	// Authenticate only if registryAuth and service are provided
	bearer := ""
	if link.registryAuth != "" && link.service != "" {
		token, err := getToken(link, "push")
		if err != nil {
			return "", err
		}

		bearer = "Bearer " + token
	}

	indexBytes, err := json.Marshal(image.index)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", PullPushProtocol, image.registry, image.repository, image.partitionTag)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(indexBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", v1.MediaTypeImageIndex)
	req.Header.Add("Authorization", bearer)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == 403 && link.registryAuth == "" {
		authHeader := response.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		fmt.Println(response.Header)
		if len(authHeader) == 0 {
			return "", fmt.Errorf("error pushing index: %d, with auth header:%v", response.StatusCode, authHeader)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		if realm == "" || service == "" {
			return "", fmt.Errorf("error pushing index: %d", response.StatusCode)
		}
		link.service = service
		link.registryAuth = realm
		return image.uploadIndex(link)
	}

	log.Println("Index Upload Status: ", response.Status)
	return bearer, nil

}

func (image *containerImage) uploadManifests() error {
	for i, manifest := range image.index.Manifests {
		manifestDigest := manifest.Digest.Encoded()
		configDigest := image.manifests[i].Config.Digest.Encoded()
		image.postByDigest(v1.MediaTypeImageManifest, manifestDigest)
		image.postByDigest(v1.MediaTypeImageManifest, configDigest)

		//TODO: use reader to perform upload
	}
	return nil
}

func (e *containerImage) uploadBlobs() error {
	return nil
}

func (image *containerImage) postByDigest(mediaType string, digest string) error {
	switch mediaType {
	case v1.MediaTypeImageManifest:
		log.Default().Println("Upload Manifest")
		manifestReader, err := image.blobCache.Get(digest)
		if err != nil {
			return err
		}
		defer manifestReader.Close()
		return nil
	case v1.MediaTypeImageConfig:
		log.Default().Println("Upload Config")
		manifestReader, err := image.blobCache.Get(digest)
		if err != nil {
			return err
		}
		defer manifestReader.Close()
		return nil
	default:
		log.Default().Println("Blob")
	}
	return nil
}
