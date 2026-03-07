package compress

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/opencontainers/go-digest"
)

type StargzCompressionResult struct {
	CompressedBlob *estargz.Blob
	TOCDigest      digest.Digest
}

func TarToStargz(tarPath string, chunkSize int, compressionLevel int) (*StargzCompressionResult, error) {
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open tar file: %w", err)
	}
	defer tarFile.Close()

	stat, err := tarFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat tar file: %w", err)
	}

	sr := io.NewSectionReader(tarFile, 0, stat.Size())

	opts := []estargz.Option{
		estargz.WithChunkSize(chunkSize),
		estargz.WithIncludeLandmarks(false),
		estargz.WithCompressionLevel(compressionLevel),
	}

	start := time.Now()
	blob, err := estargz.Build(sr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build estargz blob: %w", err)
	}
	log.Printf("estargz.Build took %s", time.Since(start))

	tocDigest := blob.TOCDigest()

	return &StargzCompressionResult{
		CompressedBlob: blob,
		TOCDigest:      tocDigest,
	}, nil
}

func CalculateStargzDigest(reader io.Reader) (string, int64, error) {
	hasher := digest.SHA256.Digester()
	size, err := io.Copy(hasher.Hash(), reader)
	if err != nil {
		return "", 0, err
	}
	return hasher.Digest().Encoded(), size, nil
}
