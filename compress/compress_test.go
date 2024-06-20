package compress

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestCompress(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create some test files in the temporary directory
	testFiles := []struct {
		name    string
		content string
	}{
		{"file1.txt", "Hello, World!"},
		{"file2.txt", "This is a test."},
	}

	for _, tf := range testFiles {
		filePath := filepath.Join(tempDir, tf.name)
		err := ioutil.WriteFile(filePath, []byte(tf.content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Generate the blob from the temporary directory
	dstFileName, err := CompressFolder(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	dstFile, err := os.Open(dstFileName)
	if err != nil {
		t.Errorf("Compressed file open error: %v", err)
	}

	// Verify that the generated blob exists
	_, err = os.Stat(dstFile.Name())
	if err != nil {
		t.Errorf("Generated blob does not exist: %v", err)
	}

	// Verify that the generated blob exists
	_, err = os.Open(dstFile.Name())
	if err != nil {
		t.Errorf("%v", err)
	}
}

func TestDecompress(t *testing.T) {

	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create some test files in the temporary directory
	testFiles := []struct {
		name    string
		content string
	}{
		{"file1.txt", "Hello, World!"},
		{"file2.txt", "This is a test."},
	}

	for _, tf := range testFiles {
		filePath := filepath.Join(tempDir, tf.name)
		err := ioutil.WriteFile(filePath, []byte(tf.content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Generate the blob from the temporary directory
	dstFile, err := CompressFolder(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a temporary directory for decompressing the blob
	tempDir2, err := ioutil.TempDir("", "test2")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Decompress the blob to the temporary directory
	err = DecompressFolder(dstFile, tempDir2)
	if err != nil {
		t.Fatal(err)
	}

	//check testfiles
	for _, tf := range testFiles {
		filePath := filepath.Join(tempDir2, tf.name)
		tmpfile, err := os.Open(filePath)
		if err != nil {
			t.Errorf("Tmp file open error: %v", err)
		}
		buffer := make([]byte, 1024)
		n, _ := tmpfile.Read(buffer)
		if string(buffer[:n]) != tf.content {
			t.Errorf("Unexpected content in destination file: got %s, want %s", string(buffer), tf.content)
		}
	}
}

func TestCalculateSha256Digest(t *testing.T) {
	// Create a temporary file for testing
	tempFile, err := ioutil.TempFile("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Write some content to the temporary file
	content := "Hello, World!"
	_, err = tempFile.WriteString(content)
	if err != nil {
		t.Fatal(err)
	}

	// Calculate the SHA256 digest of the temporary file
	tempFile.Seek(0, 0)
	digest := CalculateSha256Digest(tempFile)

	// Verify that the calculated digest matches the expected digest
	expectedDigest := sha256.Sum256([]byte(content))
	if digest != fmt.Sprintf("%x", expectedDigest) {
		t.Errorf("Unexpected SHA256 digest: got %s, want %s", digest, expectedDigest)
	}
}

func TestCopyFile(t *testing.T) {
	// Create a temporary file for testing
	srcFile, err := ioutil.TempFile("", "src")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(srcFile.Name())

	// Write some content to the source file
	content := "Hello, World!"
	_, err = srcFile.WriteString(content)
	if err != nil {
		t.Fatal(err)
	}

	// Create a temporary directory for testing
	dstDir, err := ioutil.TempDir("", "dst")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dstDir)

	// Copy the source file to the destination directory
	dstPath := filepath.Join(dstDir, "dst.txt")
	dstf, err := os.Create(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dstf.Close()
	err = CopyFile(srcFile, dstf)
	if err != nil {
		t.Fatal(err)
	}

	// Read the content of the destination file
	dstContent, err := ioutil.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the content of the destination file matches the source file
	if string(dstContent) != content {
		t.Errorf("Unexpected content in destination file: got %s, want %s", string(dstContent), content)
	}
}
