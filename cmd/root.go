package cmd

import (
	"fmt"
	"log"
	"os"
	"path"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "tdfs",
		Short: "Build a a 2dfs field ",
		Long:  `Requires a 2dfs.yaml file in the current directory or a path to a 2dfs.yaml file. Read docs at https://github.com/giobart/2dfs-builder`,
	}
	homeDir, _     = os.UserHomeDir()
	basePath       = path.Join(homeDir, ".2dfs")
	BlobStorePath  = path.Join(basePath, "blobs")
	IndexStorePath = path.Join(basePath, "index")
	KeysStorePath  = path.Join(basePath, "uncompressed-keys")
)

func Execute() error {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func init() {

	// Create a new logger with the custom format
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// check if basePath exists if not create it

	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		fmt.Print("Creating base path: " + basePath + "\n")
		err := os.Mkdir(basePath, 0755)
		if err != nil {
			log.Fatalf("Error creating base path: %v", err)
		}
	}

	//check if basePath/blobstore exists if not create it
	if _, err := os.Stat(BlobStorePath); os.IsNotExist(err) {
		os.Mkdir(BlobStorePath, 0755)
	}

	//check if basePath/index exists if not create it
	if _, err := os.Stat(KeysStorePath); os.IsNotExist(err) {
		os.Mkdir(KeysStorePath, 0755)
	}

	//check if basePath/index exists if not create it
	if _, err := os.Stat(IndexStorePath); os.IsNotExist(err) {
		os.Mkdir(IndexStorePath, 0755)
	}
}
