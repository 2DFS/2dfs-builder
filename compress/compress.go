package compress

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// GenBlobrFromPath generates an oci tar+gz from a given folder, and stores the result to toDir location
// it returns the blob digest, which is the filename, or an error
func GenBlobFromPath(fromPath string, toDir string) (string, error) {

	tmpfilename := sha256.Sum256([]byte(fromPath))

	// Open the output file for writing in gzip format
	outFile, err := os.CreateTemp(toDir, fmt.Sprintf("%x", tmpfilename))
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
	err = filepath.Walk(fromPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the source directory itself
		if path == fromPath {
			return nil
		}

		// Skip non regular files
		if !info.Mode().IsRegular() {
			return nil
		}

		// Skip the output file
		if path == outFile.Name() {
			return nil
		}

		// Create a tar header for the current file/directory
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Set the path within the tar archive relative to the source directory
		header.Name = strings.TrimPrefix(strings.Replace(path, fromPath, "", -1), string(filepath.Separator))

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
		return "", fmt.Errorf("failed walking directory: %w", err)
	}

	// Flush the gzip writer
	tarWriter.Flush()
	gzipWriter.Flush()
	err = tarWriter.Close()
	if err != nil {
		return "", fmt.Errorf("failed flushing tar file: %w", err)
	}
	err = gzipWriter.Close()
	if err != nil {
		return "", fmt.Errorf("failed flushing gzip file: %w", err)
	}

	// Calculate the sha256 digest of the file
	digest := calculateSha256Digest(outFile)
	if digest == "" {
		return "", fmt.Errorf("failed calculating sha256 digest")
	}

	// Copy the file to the final destination
	err = copyFile(outFile, filepath.Join(toDir, digest))
	if err != nil {
		return "", fmt.Errorf("failed copying tmp file to destination: %w", err)
	}

	// Remove tmp file
	_ = os.Remove(outFile.Name())

	return digest, nil
}

func calculateSha256Digest(outFile *os.File) string {
	allbytes := make([]byte, 0)
	buffer := make([]byte, 500)

	outFile.Seek(0, 0)
	for {
		n, err := outFile.Read(buffer)
		allbytes = append(allbytes, buffer[:n]...)
		if err != nil {
			break
		}
	}
	if len(allbytes) == 0 {
		return ""
	}
	digest := sha256.Sum256(allbytes)
	return fmt.Sprintf("%x", digest)
}

func copyFile(src *os.File, dst string) error {

	// Create destination file
	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	// Copy content from source file to destination file
	buffer := make([]byte, 500)
	src.Seek(0, 0)
	for {
		n, err := src.Read(buffer)
		destinationFile.Write(buffer[:n])
		if err != nil {
			break
		}
	}
	return nil

}
