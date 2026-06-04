package cache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

func createTestTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	header := &tar.Header{
		Name: name,
		Mode: 0600,
		Size: int64(len(content)),
	}

	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}

	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestBlobTagNormalizesDigest(t *testing.T) {
	got := blobTag("sha256:abc123")
	want := "blob-sha256-abc123"

	t.Logf("TEST: BlobTagNormalizesDigest")
	t.Logf("input digest: %s", "sha256:abc123")
	t.Logf("expected tag: %s", want)
	t.Logf("output tag: %s", got)

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBlobTagNormalizesDashedDigest(t *testing.T) {
	got := blobTag("sha256-abc123")
	want := "blob-sha256-abc123"

	t.Logf("TEST: BlobTagNormalizesDashedDigest")
	t.Logf("input digest: %s", "sha256-abc123")
	t.Logf("expected tag: %s", want)
	t.Logf("output tag: %s", got)

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCreateBlobImageCreatesSingleLayerImage(t *testing.T) {
	blob := createTestTarGz(t, "hello.txt", []byte("hello 2dfs"))
	compressedSha := sha256Hex(blob)

	t.Logf("TEST: CreateBlobImageCreatesSingleLayerImage")
	t.Logf("input file name: %s", "hello.txt")
	t.Logf("input file content: %s", "hello 2dfs")
	t.Logf("input blob size: %d bytes", len(blob))
	t.Logf("expected compressed sha: %s", compressedSha)

	c := &remoteCache{
		registryURL: "example.com",
		repository:  "project/cache",
	}

	img, cleanup, err := c.createBlobImage(compressedSha, bytes.NewReader(blob))
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("createBlobImage failed: %v", err)
	}

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("failed to read image layers: %v", err)
	}

	t.Logf("expected layer count: %d", 1)
	t.Logf("output layer count: %d", len(layers))

	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(layers))
	}

	layerDigest, err := layers[0].Digest()
	if err != nil {
		t.Fatalf("failed to read layer digest: %v", err)
	}

	t.Logf("expected digest algorithm: %s", "sha256")
	t.Logf("output digest algorithm: %s", layerDigest.Algorithm)
	t.Logf("expected layer digest hex: %s", compressedSha)
	t.Logf("output layer digest hex: %s", layerDigest.Hex)

	if layerDigest.Algorithm != "sha256" {
		t.Fatalf("expected sha256 algorithm, got %s", layerDigest.Algorithm)
	}

	if layerDigest.Hex != compressedSha {
		t.Fatalf("expected layer digest %s, got %s", compressedSha, layerDigest.Hex)
	}

	rc, err := layers[0].Compressed()
	if err != nil {
		t.Fatalf("failed to read compressed layer: %v", err)
	}
	defer rc.Close()

	layerBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("failed to read compressed layer bytes: %v", err)
	}

	t.Logf("expected compressed layer size: %d bytes", len(blob))
	t.Logf("output compressed layer size: %d bytes", len(layerBytes))
	t.Logf("expected layer bytes equal original blob: %t", true)
	t.Logf("output layer bytes equal original blob: %t", bytes.Equal(layerBytes, blob))
	logTarGzContents(t, layerBytes)

	if !bytes.Equal(layerBytes, blob) {
		t.Fatalf("compressed layer bytes differ from original blob")
	}
}

func TestCreateBlobImageRejectsWrongDigest(t *testing.T) {
	blob := createTestTarGz(t, "hello.txt", []byte("hello 2dfs"))
	wrongDigest := "deadbeef"
	actualDigest := sha256Hex(blob)

	t.Logf("TEST: CreateBlobImageRejectsWrongDigest")
	t.Logf("input file name: %s", "hello.txt")
	t.Logf("input file content: %s", "hello 2dfs")
	t.Logf("input blob size: %d bytes", len(blob))
	t.Logf("input wrong digest: %s", wrongDigest)
	t.Logf("actual blob digest: %s", actualDigest)
	t.Logf("expected result: error")

	c := &remoteCache{
		registryURL: "example.com",
		repository:  "project/cache",
	}

	_, cleanup, err := c.createBlobImage(wrongDigest, bytes.NewReader(blob))
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatalf("expected digest mismatch error, got nil")
	}
}

func logTarGzContents(t *testing.T, data []byte) {
	t.Helper()

	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read tar entry: %v", err)
		}

		content, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatalf("failed to read tar entry content: %v", err)
		}

		t.Logf("layer tar entry name: %s", header.Name)
		t.Logf("layer tar entry size: %d bytes", header.Size)
		t.Logf("layer tar entry content: %s", string(content))
	}
}
