package cmd

import (
	"os"
	"strings"

	"github.com/giobart/2dfs-builder/cache"
	"github.com/giobart/2dfs-builder/oci"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(imageCmd)
	imageCmd.AddCommand(imageListCmd)
	imageListCmd.Flags().BoolVarP(&showHash, "hash", "q", false, "config file (default is .2dfs.json)")
}

var showHash bool
var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Commands to manage images",
}
var imageListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List local images",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listImages()
	},
}

var tableStyle = table.Style{
	Name: "style1",
	Box: table.BoxStyle{
		MiddleHorizontal: "-",
		PaddingLeft:      " ",
		PaddingRight:     " ",
	},
	Options: table.Options{
		DrawBorder:      false,
		SeparateColumns: false,
		SeparateFooter:  false,
		SeparateHeader:  true,
		SeparateRows:    false,
	},
}

func listImages() error {

	indexCacheStore, err := cache.NewCacheStore(IndexStorePath)
	if err != nil {
		return err
	}
	indexHashList := indexCacheStore.List()
	if showHash {
		for _, hash := range indexHashList {
			println(hash)
		}
		return nil
	}

	blobCacheStore, err := cache.NewCacheStore(BlobStorePath)
	if err != nil {
		return err
	}

	outTable := table.NewWriter()
	outTable.SetOutputMirror(os.Stdout)
	outTable.AppendHeader(table.Row{"#", "Name", "Tag", "Type"})
	outTable.AppendSeparator()

	for i, hash := range indexHashList {
		reader, err := indexCacheStore.Get(hash)
		if err != nil {
			return err
		}
		idx, err := oci.ReadIndex(reader)
		reader.Close()
		if err != nil {
			return err
		}

		imageType := "OCI"

		firstManifestDigest := idx.Manifests[0].Digest.Encoded()
		blobReader, err := blobCacheStore.Get(firstManifestDigest)
		if err != nil {
			return err
		}
		manifest, err := oci.ReadManifest(blobReader)
		blobReader.Close()
		if err != nil {
			return err
		}
		for _, l := range manifest.Layers {
			if l.MediaType == oci.TwoDfsMediaType {
				imageType = "OCI+2DFS"
				break
			}
		}
		imageUrl := idx.Manifests[0].Annotations["org.opencontainers.image.url"]
		//keep only last part of the url
		imageUrl = imageUrl[strings.LastIndex(imageUrl, "/")+1:]
		imageTag := idx.Manifests[0].Annotations["org.opencontainers.image.version"]
		outTable.AppendRow([]interface{}{i, imageUrl, imageTag, imageType})
	}

	outTable.SetStyle(tableStyle)
	outTable.Render()

	return nil
}
