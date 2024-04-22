package compress

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// GenBlobrFromPath generates an oci tar+gz from a given folder, and stores the result to toDir location
// it returns the blob digest, which is the filename, or an error
func GenBlobFromPath(fromDir string, toDir string) (string, error) {
	os.CreateTemp(toDir, "tempfile")

	// Open the output file for writing in gzip format
	outFile, err := os.Create(targetFile)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Create a new tar archive writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Walk through the source directory
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the source directory itself
		if path == sourceDir {
			return nil
		}

		// Create a tar header for the current file/directory
		header, err := tar.FileInfoHeader(info, info.Size())
		if err != nil {
			return err
		}

		// Set the path within the tar archive relative to the source directory
		header.Name = filepath.ToSlash(filepath.Rel(sourceDir, path))

		// Write the header to the tar archive
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// If it's a regular file, open it and copy the content to the tar archive
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(tarWriter, file)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed walking directory: %w", err)
	}

	// Flush and close the tar archive
	return tarWriter.Close()
}
