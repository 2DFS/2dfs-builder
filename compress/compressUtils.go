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
	"time"
)

// Efficiently Tar and Gzip a folder
func CompressFolder(fromPath string) (string, error) {

	tmpfilename := sha256.Sum256([]byte(fromPath))
	tmpdir := os.TempDir()

	// Open the output file for writing in gzip format
	outFile, err := os.CreateTemp(tmpdir, fmt.Sprintf("%x", tmpfilename))
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
	copyBuffer := make([]byte, 1024*1024)
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
		header.AccessTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		header.ChangeTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		header.ModTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

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

			_, err = io.CopyBuffer(tarWriter, file, copyBuffer)
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

	return outFile.Name(), nil
}

// Creates a tar from a folder
func TarFolder(fromPath string) (string, error) {

	tmpfilename := sha256.Sum256([]byte(fromPath))
	tmpdir := os.TempDir()

	// Open the output file for writing in gzip format
	outFile, err := os.CreateTemp(tmpdir, fmt.Sprintf("%x", tmpfilename))
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	// Create a new tar archive writer
	tarWriter := tar.NewWriter(outFile)
	defer tarWriter.Close()

	// Walk through the source directory
	copyBuffer := make([]byte, 1024*1024)
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
		header.AccessTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		header.ChangeTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		header.ModTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

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

			_, err = io.CopyBuffer(tarWriter, file, copyBuffer)
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
	err = tarWriter.Close()
	if err != nil {
		return "", fmt.Errorf("failed flushing tar file: %w", err)
	}

	return outFile.Name(), nil
}

// Creates a tar from a file
func TarFile(src []string, dst []string) (string, error) {
	if len(src) != len(dst) {
		return "", fmt.Errorf("src and Dst list size do not match: %d!=%d", len(src), len(dst))
	}

	// Create a new tar archive writer
	tmpdir := os.TempDir()

	// Open the output file for writing in tar format
	outFile, err := os.CreateTemp(tmpdir, "*")
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	tarWriter := tar.NewWriter(outFile)
	defer tarWriter.Close()

	for i, s := range src {

		// Walk through the source directory
		copyBuffer := make([]byte, 1024*1024)
		info, err := os.Stat(s)
		if err != nil {
			return "", err
		}

		// Skip non regular files
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("The input is a non-regular file")
		}

		// Create a tar header for the current file/directory
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return "", err
		}
		header.AccessTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		header.ChangeTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		header.ModTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

		// Set the path within the tar archive to file name
		header.Name = dst[i]

		// Write the header to the tar archive
		if err := tarWriter.WriteHeader(header); err != nil {
			return "", err
		}

		// copy file inside tar
		err = func() error {
			file, err := os.Open(s)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.CopyBuffer(tarWriter, file, copyBuffer)
			if err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			return "", nil
		}

		// Flush the writer
		tarWriter.Flush()

	}

	err = tarWriter.Close()
	if err != nil {
		return "", fmt.Errorf("failed flushing tar file: %w", err)
	}

	return outFile.Name(), nil
}

// Applies a gzip compression to a tar file
func TarToGz(tarFilePath string) (string, error) {

	// Open the output file for writing in gzip format
	outFile, err := os.CreateTemp(os.TempDir(), "tar.gz")
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Open the tar file
	tarFile, err := os.Open(tarFilePath)
	if err != nil {
		return "", err
	}
	defer tarFile.Close()

	_, err = io.Copy(gzipWriter, tarFile)
	if err != nil {
		return "", err
	}

	return outFile.Name(), nil
}

func DecompressFolder(targzFilePath string, outputDirectory string) error {

	// Open given tar file
	targzfile, err := os.Open(targzFilePath)
	if err != nil {
		return err
	}

	// Open gz reader
	gzipReader, err := gzip.NewReader(targzfile)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	// Create a new tar archive reader
	tarReader := tar.NewReader(gzipReader)

	copyBuffer := make([]byte, 1024*1024)
	// Walk through the tar archive
	for {
		header, err := tarReader.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it
		case header == nil:
			continue
		}

		target := filepath.Join(outputDirectory, header.Name)

		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.CopyBuffer(f, tarReader, copyBuffer); err != nil {
				return err
			}
			f.Close()
		}
	}
}

func CalculateSha256Digest(outFile io.ReadCloser) string {
	hash := sha256.New()
	size, err := io.Copy(hash, outFile)
	if err != nil || size == 0 {
		return ""
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func CalculateMultiSha256Digest(multifile []string) (string, error) {
	digests := []byte{}
	for _, f := range multifile {
		err := func() error {
			reader, err := os.Open(f)
			if err != nil {
				return err
			}
			defer reader.Close()
			digests = append(digests, []byte(CalculateSha256Digest(reader))...)
			return nil

		}()
		if err != nil {
			return "", err
		}
	}

	digest := sha256.Sum256(digests)
	return fmt.Sprintf("%x", digest), nil
}

func CopyFile(src *os.File, dst *os.File) error {

	// Copy content from source file to destination file
	buffer := make([]byte, 500)
	src.Seek(0, 0)
	for {
		n, err := src.Read(buffer)
		dst.Write(buffer[:n])
		if err != nil {
			break
		}
	}
	return nil

}
