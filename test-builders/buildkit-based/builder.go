package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/2DFS/2dfs-builder/filesystem"
	buildkitclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/imagemetaresolver"
	"github.com/moby/buildkit/client/llb/sourceresolver"
	gatewayclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session/filesync"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/tonistiigi/fsutil"
)

type buildOpt struct {
	baseImage             string
	twodfsFile            string
	buildkitClientAddress string
}

type TwoDFsFile struct {
	Entry []Entry `json:"allotments"`
}

type Entry struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
	Row int    `json:"row"`
	Col int    `json:"col"`
}

type TwoDFileSystemEntry struct {
	Row    int
	Col    int
	Digest string
}

type TwoDFileSystem struct {
}

func main() {
	var opt buildOpt
	flag.StringVar(&opt.baseImage, "i", "", "Base image to use for the build")
	flag.StringVar(&opt.twodfsFile, "f", "./manifest.json", "2dfs manifest file")
	flag.StringVar(&opt.buildkitClientAddress, "c", "unix:///run/buildkit/buildkitd.sock", "custom buildkitd socket")
	flag.Parse()

	ctx := context.Background()

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	//append images/ to cwd using filepath.Join
	imagesFolder := filepath.Join(cwd, "images")
	//create images folder if it does not exist
	if _, err := os.Stat(imagesFolder); os.IsNotExist(err) {
		os.Mkdir(imagesFolder, 0755)
	}

	buildClient, err := ResolveClient(opt.buildkitClientAddress)
	if err != nil {
		log.Fatal(err)
	}

	fsFile, err := Parse(opt.twodfsFile)
	if err != nil {
		log.Fatal(err)
	}

	constraints := llb.NewConstraints()
	out := GenLLB(ctx, opt.baseImage, fsFile, constraints)

	dt, err := out.Marshal(context.TODO())
	if err != nil {
		panic(err)
	}
	//llb.WriteTo(dt, os.Stdout)
	log.Default().Println("LLB Ready... Solving now")
	fsdir, err := fsutil.NewFS(".")
	if err != nil {
		panic(err)
	}

	product := "2DFS_test"
	outtar := filepath.Join(imagesFolder, "image.tar")
	outW, err := os.Create(outtar)
	outW.Chmod(0755)
	if err != nil {
		log.Fatal(err)
	}

	solveopts := buildkitclient.SolveOpt{
		Exports: []buildkitclient.ExportEntry{
			{
				Type:   buildkitclient.ExporterOCI,
				Output: fixedWriteCloser(outW),
				Attrs: map[string]string{
					"attestation-prefix": "test.",
				},
			},
		},
		//mount local folder
		LocalMounts: map[string]fsutil.FS{
			"context": fsdir,
		},
	}

	buildFunc := func(ctx context.Context, c gatewayclient.Client) (*gatewayclient.Result, error) {
		r, err := c.Solve(ctx, gatewayclient.SolveRequest{
			Definition: dt.ToPB(),
		})

		log.Default().Println("Solved")
		log.Default().Println(r)
		if err != nil {
			log.Fatal(err)
		}
		return r, nil
	}
	log.Default().Println("Building...")
	response, err := buildClient.Build(ctx, solveopts, product, buildFunc, nil)
	if err != nil {
		panic(err)
	}
	print(response.ExporterResponse)
	printMetadataFile(response.ExporterResponse)
}

func Parse(filePath string) (TwoDFsFile, error) {

	// Read 2dfs manifest from disk
	data, err := os.ReadFile(filePath)
	if err != nil {
		return TwoDFsFile{}, err
	}

	// Create an empty struct to hold the decoded data
	var jsonData TwoDFsFile

	// Unmarshal the JSON data into the struct
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return TwoDFsFile{}, err
	}

	return jsonData, nil

}

// GenLLB Generates LLB language from 2dfs manifest */
func GenLLB(ctx context.Context, baseImage string, fs TwoDFsFile, constraints *llb.Constraints) llb.State {
	var states []llb.State //pointers to all generated allotments

	base := llb.Image(baseImage)
	field := filesystem.GetField()

	_, _, dt, err := imagemetaresolver.Default().ResolveImageConfig(ctx, baseImage, sourceresolver.Opt{})
	if err != nil {
		log.Fatal(err)
	}
	var img ocispecs.Image
	if err := json.Unmarshal(dt, &img); err != nil {
		log.Fatal(err)
	}

	for _, allotment := range fs.Entry {

		// Generating allotment option to add 2dfs description to oci layer
		allotmentToString, _ := json.Marshal(allotment)
		allotmentOpt := llb.WithDescription(map[string]string{"2dfs-allotment": string(allotmentToString)})

		// Generating layer for this allotment and saving the state
		fileaction := llb.Copy(llb.Local("context"), allotment.Src, allotment.Dst)
		allotmentstate := llb.Merge([]llb.State{base, llb.Scratch().File(fileaction, allotmentOpt)}, llb.WithDescription(map[string]string{"2dfs-allotment": fmt.Sprintf("%v", allotment)}))
		if err != nil {
			log.Fatal(err)
		}
		digest, _, _, _, _ := allotmentstate.Output().Vertex(ctx, constraints).Marshal(ctx, constraints)
		//newstate := base.WithOutput(allotmentLayer)
		states = append(states, allotmentstate)
		//storing layer digest as part of allotment information
		if err != nil {
			log.Fatal(err)
		}
		field.AddAllotment(filesystem.Allotment{
			Row:    allotment.Row,
			Col:    allotment.Col,
			Digest: digest.String(),
		})
	}

	// Merging layers and marshalling filesystem as layer config in a new image
	finalState := llb.Merge(states)
	log.Default().Println(field.Marshal())
	//finalState := base.WithOutput(llb.Merge(states, twodfsOpt).Output())

	//twodfsImage, err := finalState.WithImageConfig(imageConfig)
	return finalState
}

func ResolveClient(address string) (*buildkitclient.Client, error) {
	return buildkitclient.New(context.Background(), address)
}

func fixedWriteCloser(wc io.WriteCloser) filesync.FileOutputFunc {
	return func(map[string]string) (io.WriteCloser, error) {
		return wc, nil
	}
}

func printMetadataFile(exporterResponse map[string]string) {
	for k, v := range exporterResponse {
		log.Default().Printf(fmt.Sprintf("%s,%s\n", k, v))
	}
}
