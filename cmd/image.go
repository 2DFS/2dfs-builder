package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/giobart/2dfs-builder/cache"
	"github.com/giobart/2dfs-builder/filesystem"
	"github.com/giobart/2dfs-builder/oci"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(imageCmd)
	imageCmd.AddCommand(imageListCmd)
	imageListCmd.Flags().BoolVarP(&showHash, "reference", "q", false, "returns only the refrerence list")
	imageCmd.AddCommand(rm)
	rm.Flags().BoolVarP(&removeAll, "all", "a", false, "removes all images")
	imageCmd.AddCommand(prune)
	imageCmd.AddCommand(export)
	export.Flags().StringVar(&exportFormat, "as", "", "export format, supported formats: tar")
}

var showHash bool
var removeAll bool
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
var rm = &cobra.Command{
	Use:   "rm [reference]...",
	Short: "remove local images",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return removeImages(args)
	},
}

var prune = &cobra.Command{
	Use:   "prune",
	Short: "clean unreferenced cache entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		return pruneBlobs()
	},
}

var export = &cobra.Command{
	Use:   "export [reference] [targetFile]",
	Short: "export image to target file. E.g. export [imgref] MyImage.tar.gz",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return imageExport(args[0], args[1])
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
	outTable.AppendHeader(table.Row{"#", "Url", "Tag", "Type", "Reference"})
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
		imageUrl := idx.Annotations[oci.ImageNameAnnotation]
		//keep only last part of the url
		imageTag := idx.Manifests[0].Annotations["org.opencontainers.image.version"]
		outTable.AppendRow([]interface{}{i, imageUrl, imageTag, imageType, hash})
	}

	outTable.SetStyle(tableStyle)
	outTable.Render()

	return nil
}

func removeImages(args []string) error {
	indexCacheStore, err := cache.NewCacheStore(IndexStorePath)
	if err != nil {
		return err
	}
	if removeAll {
		args = indexCacheStore.List()
	}
	//remove index
	for _, arg := range args {
		indexCacheStore.Del(arg)
		indexCacheStore.Del(fmt.Sprintf("%x", sha256.Sum256([]byte(arg))))
	}
	pruneBlobs()

	return nil
}

// pruneBlobs removes blobs that are not referenced by any index
func pruneBlobs() error {
	indexCacheStore, err := cache.NewCacheStore(IndexStorePath)
	if err != nil {
		return err
	}
	blobCacheStore, err := cache.NewCacheStore(BlobStorePath)
	if err != nil {
		return err
	}

	//create reference counter for blobs
	blobs := blobCacheStore.List()
	blobreferences := make(map[string]int)
	for _, blob := range blobs {
		blobreferences[blob] = 0
	}
	indexes := indexCacheStore.List()
	for _, index := range indexes {
		reader, err := indexCacheStore.Get(index)
		if err != nil {
			return err
		}
		idx, err := oci.ReadIndex(reader)
		reader.Close()
		if err != nil {
			return err
		}

		//add reference for each layer,manifest,config and allotment file referenced by the index
		for _, m := range idx.Manifests {
			blobreferences[m.Digest.Encoded()]++
			manifestReader, err := blobCacheStore.Get(m.Digest.Encoded())
			if err != nil {
				return err
			}
			manifest, err := oci.ReadManifest(manifestReader)
			manifestReader.Close()
			if err != nil {
				return err
			}
			for _, l := range manifest.Layers {
				blobreferences[l.Digest.Encoded()]++
				if l.MediaType == oci.TwoDfsMediaType {
					tdfsReader, err := blobCacheStore.Get(l.Digest.Encoded())
					if err != nil {
						return err
					}
					fieldBytes, err := io.ReadAll(tdfsReader)
					tdfsReader.Close()
					if err != nil {
						return err
					}
					tdfs, err := filesystem.GetField().Unmarshal(string(fieldBytes))
					if err != nil {
						return err
					}
					for f := range tdfs.IterateAllotments() {
						blobreferences[f.Digest]++
					}
				}
			}
			blobreferences[manifest.Config.Digest.Encoded()]++
		}

	}
	//garbage collect unreferenced blobs
	removed := 0
	for blob, ref := range blobreferences {
		if ref == 0 {
			blobCacheStore.Del(blob)
			fmt.Printf("%s [REMOVED]\n", blob)
			removed++
		}
	}
	fmt.Println("Removed", removed, "blobs")
	return nil
}

func imageExport(reference string, dstFile string) error {
	timestart := time.Now().UnixMilli()

	ctx := context.Background()
	ctx = context.WithValue(ctx, oci.IndexStoreContextKey, IndexStorePath)
	ctx = context.WithValue(ctx, oci.BlobStoreContextKey, BlobStorePath)
	log.Default().Printf("Retrieving %s from local cache...\n", reference)
	ociImage, err := oci.GetLocalImage(ctx, reference)
	if err != nil {
		return err
	}

	log.Default().Printf("Exporting %s to %s...\n", reference, dstFile)
	err = ociImage.GetExporter().ExportAsTar(dstFile)
	if err != nil {
		return err
	}

	timeend := time.Now().UnixMilli()
	totTime := timeend - timestart
	timeS := float64(float64(totTime) / 1000)

	log.Default().Printf("Done!  ✅ (%fs)\n", timeS)

	return nil
}
