package cmd

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"time"

	"github.com/2DFS/2dfs-builder/filesystem"
	"github.com/2DFS/2dfs-builder/oci"
	"github.com/spf13/cobra"
)

func init() {
	buildCmd.Flags().StringVarP(&buildFile, "file", "f", "2dfs.json", "2dfs manifest file")
	buildCmd.Flags().StringVar(&exportFormat, "as", "", "export format, supported formats: tar")
	buildCmd.Flags().BoolVar(&forcePull, "force-pull", false, "force pull the base image")
	buildCmd.Flags().BoolVar(&forceHttp, "force-http", false, "force pull via http")
	buildCmd.Flags().StringArrayVarP(&platfrorms, "platforms", "p", []string{}, "Filter the build platoforms. E.g. linux/amd64,linux/arm64. By default all the available platforms are used")
	buildCmd.Flags().BoolVar(&enableStargz, "enable-stargz", false, "enable stargz compression for allotments")
	buildCmd.Flags().IntVar(&stargzChunkSize, "stargz-chunk-size", 1024*1024, "chunk size for stargz compression in bytes")
	buildCmd.Flags().IntVar(&stargzCompressionLevel, "stargz-compression-level", gzip.BestSpeed, "gzip compression level for stargz (1=fastest, 9=smallest)")
	rootCmd.AddCommand(buildCmd)
}

var buildFile string
var forcePull bool
var forceHttp bool
var exportFormat string
var platfrorms []string
var enableStargz bool
var stargzChunkSize int
var stargzCompressionLevel int
var buildCmd = &cobra.Command{
	Use:   "build [base image] [target image]",
	Short: "Build a 2dfs field from an oci image link",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return build(args[0], args[1])
	},
}

func build(imgFrom string, imgTarget string) error {
	timestart := time.Now().UnixMilli()

	bf, err := os.Open(buildFile)
	if err != nil {
		return err
	}
	defer bf.Close()

	//parse bf json file as filesystem.TwoDFsManifest
	log.Default().Println("Parsing manifest file")
	twoDfsManifest := filesystem.TwoDFsManifest{}
	bytes, err := io.ReadAll(bf)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &twoDfsManifest)
	if err != nil {
		return err
	}
	log.Default().Println("Manifest parsed")

	// build the 2dfs field
	ctx := context.Background()
	ctx = context.WithValue(ctx, oci.IndexStoreContextKey, IndexStorePath)
	ctx = context.WithValue(ctx, oci.BlobStoreContextKey, BlobStorePath)
	ctx = context.WithValue(ctx, oci.KeyStoreContextKey, KeysStorePath)
	log.Default().Println("Getting Image")
	oci.PullPushProtocol = "https"
	if forceHttp {
		oci.PullPushProtocol = "http"
	}
	stargzOptions := oci.StargzOptions{
		Enabled:          enableStargz,
		ChunkSize:        stargzChunkSize,
		CompressionLevel: stargzCompressionLevel,
	}
	ociImage, err := oci.NewImageWithStargzOptions(ctx, imgFrom, forcePull, platfrorms, stargzOptions)
	if err != nil {
		return err
	}

	// add 2dfs field to the image
	buildstart := time.Now().UnixMilli()
	log.Default().Println("Adding Field")
	err = ociImage.AddField(twoDfsManifest, imgTarget)
	if err != nil {
		return err
	}
	log.Default().Println("Field Added")

	// export the image is "as" was set
	if exportFormat != "" {
		switch exportFormat {
		case "tar":
			exporter, err := ociImage.GetExporter()
			if err != nil {
				return err
			}
			err = exporter.ExportAsTar("image.tar.gz")
			if err != nil {
				return err
			}
		}
	}

	timeend := time.Now().UnixMilli()
	totTime := timeend - timestart
	buildTime := timeend - buildstart
	timeS := float64(float64(totTime) / 1000)
	timebuildS := float64(float64(buildTime) / 1000)

	log.Default().Printf("Build completed  ⚒️ (%fs)\n", timebuildS)
	log.Default().Printf("Done!  ✅ (%fs)\n", timeS)
	return nil
}
