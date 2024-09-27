package oci

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var homeDir string
var basePath string
var BlobStorePath string
var IndexStorePath string
var KeysStorePath string

func init() {
	homeDir, _ = os.UserHomeDir()
	basePath = path.Join(homeDir, ".2dfs")
	BlobStorePath = path.Join(basePath, "blobs")
	IndexStorePath = path.Join(basePath, "index")
	KeysStorePath = path.Join(basePath, "uncompressed-keys")
}

func TestToken(t *testing.T) {

	link := OciImageLink{
		registryAuth: "https://auth.docker.io/token",
		Registry:     "index.docker.io",
		service:      "registry.docker.io",
		Repository:   "library/ubuntu",
		Reference:    "latest",
	}

	_, err := getToken(link)

	if err != nil {
		t.Errorf("DownloadIndex error: %v", err)
	}

}

func TestDownloadIndexDocker(t *testing.T) {

	link := OciImageLink{
		Registry:   "docker.io",
		Repository: "library/nginx",
		Reference:  "latest",
	}

	index, err := DownloadIndex(link)

	if err != nil {
		t.Errorf("DownloadIndex error: %v", err)
		t.Failed()
	}

	if index.SchemaVersion != 2 {
		t.Errorf("Invalid Schems error, expected %d, given %d", 2, index.SchemaVersion)
		t.Failed()
	}

	if strings.Contains(index.Manifests[0].Annotations["org.opencontainers.image.src"], "nginx") {
		t.Errorf("not a nginx image")
		t.Failed()
	}

}

func TestDownloadIndexGhcrFail(t *testing.T) {

	link := OciImageLink{
		Registry:   "ghcr.io",
		Repository: "giobart/message-broker/message-broker",
		Reference:  "v1.2.4",
	}

	_, err := DownloadIndex(link)

	if err != nil {
		if !strings.Contains(err.Error(), "invalid index media type") {
			t.Errorf("DownloadIndex error: %v", err)
			t.Fatal()
		}
	}

}

func TestParseWWWAuth(t *testing.T) {

	// Example: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"
	auth := `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"`

	realm, service := parseWWWAuthenticate([]string{auth})

	if realm != "https://auth.docker.io/token" {
		t.Errorf("Invalid realm error, expected %s, given %s", "https://auth.docker.io/token", realm)
	}

	if service != "registry.docker.io" {
		t.Errorf("Invalid service error, expected %s, given %s", "registry.docker.io", service)
	}

}

func TestDownloadManifest(t *testing.T) {

	link := OciImageLink{
		Registry:   "docker.io",
		Repository: "library/nginx",
		Reference:  "latest",
	}

	index, err := DownloadIndex(link)

	if err != nil {
		t.Errorf("DownloadIndex error: %v", err)
		t.Failed()
	}

	manifestDigest := index.Manifests[0].Digest
	fmt.Println("Digest: ", manifestDigest.String())
	manifestReader, err := DownloadManifest(link, manifestDigest.String())
	if err != nil {
		t.Errorf("DownloadBlob error: %v", err)
		t.Failed()
		return
	}
	defer manifestReader.Close()

	manifest, _, _, err := ReadManifest(manifestReader)

	if err != nil {
		t.Errorf("ReadManifest error: %v", err)
		t.Failed()
	}

	if manifest.MediaType != v1.MediaTypeImageManifest {
		t.Errorf("Invalid manifest media type: %s", manifest.MediaType)
		t.Failed()
	}
}

func TestDownloadBase64(t *testing.T) {

	// build the 2dfs field
	ctx := context.Background()
	ctx = context.WithValue(ctx, IndexStoreContextKey, IndexStorePath)
	ctx = context.WithValue(ctx, BlobStoreContextKey, BlobStorePath)
	ctx = context.WithValue(ctx, KeyStoreContextKey, KeysStorePath)

	imgFrom := "docker.io/library/nginx:latest"

	ociImage, err := NewImage(ctx, imgFrom, false, []string{"linux/amd64"})
	if err != nil {
		t.Errorf("NewImage error: %v", err)
		t.Failed()
	}
	image := ociImage.(*containerImage)

	layer0Digest := image.manifests[0].Layers[0].Digest.Encoded()
	blobReader, err := image.blobCache.Get(layer0Digest)
	if err != nil {
		t.Errorf("NewImage error: %v", err)
		t.Failed()
	}
	defer blobReader.Close()

	base64reader := reverseBase64Reader(blobReader)
	byteReader := base64.NewDecoder(base64.StdEncoding, base64reader)

	Resultbytes, err := io.ReadAll(byteReader)
	if err != nil {
		t.Errorf("NewImage error: %v", err)
		t.Failed()
	}

	originalReader, err := image.blobCache.Get(layer0Digest)
	Expectedbytes, err := io.ReadAll(originalReader)
	if err != nil {
		t.Errorf("NewImage error: %v", err)
		t.Failed()
	}

	if string(Resultbytes) != string(Expectedbytes) {
		t.Errorf("Invalid base64 decoding")
		t.Errorf("Expected: %s...", string(Expectedbytes[:20]))
		t.Errorf("Given: %s...", string(Resultbytes[:20]))
		t.Failed()
	}

}
