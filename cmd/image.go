package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
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
	export.Flags().StringVar(&platform, "platform", "", "select platform, e.g., linux/amd64 or linux/arm64. Default: multiplatform image")
	imageCmd.AddCommand(push)
}

var showHash bool
var removeAll bool
var platform string
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

var push = &cobra.Command{
	Use:   "push [reference]",
	Short: "push image to the registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return imagePush(args[0])
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
		manifest, _, _, err := oci.ReadManifest(blobReader)
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
		//remove directory BlobStorePath
		_ = os.RemoveAll(BlobStorePath)
		//remove directory KeysStorePath
		_ = os.RemoveAll(KeysStorePath)
		//remove directory IndexStorePath
		_ = os.RemoveAll(IndexStorePath)
		return nil
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
	blobDigestCacheStore, err := cache.NewCacheStore(KeysStorePath)
	if err != nil {
		return err
	}

	//create reference counter for blobs
	blobs := blobCacheStore.List()
	digests := blobDigestCacheStore.List()
	blobreferences := make(map[string]int)
	digestreferences := make(map[string]int)

	for _, blob := range blobs {
		blobreferences[blob] = 0
	}
	for _, blodigest := range digests {
		reader, err := blobDigestCacheStore.Get(blodigest)
		if err != nil {
			return err
		}
		cachekeys, err := oci.ParseCacheKey(reader)
		if err != nil {
			return err
		}
		for _, k := range cachekeys.Keys {
			digestreferences[k.DiffID] = 0
		}
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
			manifest, _, _, err := oci.ReadManifest(manifestReader)
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
						digestreferences[f.DiffID]++
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

	//garbage collect unreferenced cache file keys
	keys := blobDigestCacheStore.List()
	for _, key := range keys {
		err := func() error {
			keyreader, err := blobDigestCacheStore.Get(key)
			if err != nil {
				return err
			}
			defer keyreader.Close()
			cachekeys, err := oci.ParseCacheKey(keyreader)
			if err != nil {
				return err
			}
			newkeys := []oci.FileCacheKey{}
			for _, k := range cachekeys.Keys {
				if digestreferences[k.DiffID] != 0 {
					newkeys = append(newkeys, k)
				}
			}
			if len(newkeys) != len(cachekeys.Keys) {
				blobDigestCacheStore.Del(key)
				fmt.Printf("%s [REMOVED]\n", key)
				if len(newkeys) >= 0 {
					newkey := oci.CacheKeys{
						Keys: newkeys,
					}
					newkeyBytes, err := json.Marshal(newkey)
					if err != nil {
						return err
					}
					newkeyReader, err := blobDigestCacheStore.Add(key)
					if err != nil {
						return err
					}
					defer newkeyReader.Close()
					_, err = newkeyReader.Write(newkeyBytes)
					if err != nil {
						return err
					}
				}
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}

	for digest, ref := range digestreferences {
		if ref == 0 {
			blobDigestCacheStore.Del(digest)
			removed++
		}
	}
	fmt.Println("Removed", removed, "blobs")
	return nil
}

func imageExport(reference string, dstFile string) error {
	os, arch := "", ""
	if platform != "" {
		splitPlat := strings.Split(platform, "/")
		if len(splitPlat) != 2 {
			return fmt.Errorf("invalid platform format")
		}
		os = splitPlat[0]
		arch = splitPlat[1]
	}
	timestart := time.Now().UnixMilli()

	ctx := context.Background()
	ctx = context.WithValue(ctx, oci.IndexStoreContextKey, IndexStorePath)
	ctx = context.WithValue(ctx, oci.BlobStoreContextKey, BlobStorePath)
	ctx = context.WithValue(ctx, oci.KeyStoreContextKey, KeysStorePath)
	log.Default().Printf("Retrieving %s from local cache...\n", reference)
	ociImage, err := oci.GetLocalImage(ctx, reference)
	if err != nil {
		return err
	}

	log.Default().Printf("Exporting %s to %s...\n", reference, dstFile)
	exporter, err := ociImage.GetExporter(os, arch)
	if err != nil {
		return err
	}
	err = exporter.ExportAsTar(dstFile)
	if err != nil {
		return err
	}

	timeend := time.Now().UnixMilli()
	totTime := timeend - timestart
	timeS := float64(float64(totTime) / 1000)

	log.Default().Printf("Done!  ✅ (%fs)\n", timeS)

	return nil
}

func imagePush(reference string) error {
	os, arch := "", ""
	if platform != "" {
		splitPlat := strings.Split(platform, "/")
		if len(splitPlat) != 2 {
			return fmt.Errorf("invalid platform format")
		}
		os = splitPlat[0]
		arch = splitPlat[1]
	}
	timestart := time.Now().UnixMilli()

	ctx := context.Background()
	ctx = context.WithValue(ctx, oci.IndexStoreContextKey, IndexStorePath)
	ctx = context.WithValue(ctx, oci.BlobStoreContextKey, BlobStorePath)
	ctx = context.WithValue(ctx, oci.KeyStoreContextKey, KeysStorePath)
	log.Default().Printf("Retrieving %s from local cache...\n", reference)
	ociImage, err := oci.GetLocalImage(ctx, reference)
	if err != nil {
		return err
	}

	log.Default().Printf("Pushing %s...\n", reference)
	exporter, err := ociImage.GetExporter(os, arch)
	if err != nil {
		return err
	}
	err = exporter.Upload()
	if err != nil {
		return err
	}

	timeend := time.Now().UnixMilli()
	totTime := timeend - timestart
	timeS := float64(float64(totTime) / 1000)

	log.Default().Printf("Done!  ✅ (%fs)\n", timeS)

	return nil
}
