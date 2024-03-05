package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/giobart/2dfs-builder/filesystem"
	"log"
	"os"

	"github.com/moby/buildkit/client/llb"
)

type buildOpt struct {
	baseImage  string
	twodfsFile string
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
	flag.Parse()

	fsFile, err := Parse(opt.twodfsFile)
	if err != nil {
		log.Fatal(err)
	}

	constraints := llb.NewConstraints()
	out := GenLLB(opt.baseImage, fsFile, constraints)

	dt, err := out.Marshal(context.TODO())
	if err != nil {
		panic(err)
	}
	llb.WriteTo(dt, os.Stdout)

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
func GenLLB(baseImage string, fs TwoDFsFile, constraints *llb.Constraints) llb.State {
	var states []llb.State //pointers to all generated allotments

	base := llb.Image(baseImage)
	field := filesystem.GetField()

	for _, allotment := range fs.Entry {

		// Generating allotment option to add 2dfs description to oci layer
		allotmentToString, _ := json.Marshal(allotment)
		allotmentOpt := llb.WithDescription(map[string]string{"2dfs-allotment": string(allotmentToString)})

		// Generating layer for this allotment and saving the state
		fileaction := llb.Copy(llb.Scratch(), allotment.Src, allotment.Dst) //, allotmentOpt)
		allotmentLayer := llb.Merge([]llb.State{base, llb.Scratch().File(fileaction, allotmentOpt)}).Output()
		newstate := base.WithOutput(allotmentLayer)
		states = append(states, newstate)

		//storing layer digest as part of allotment information
		digest, err := allotmentLayer.ToInput(context.Background(), constraints)
		if err != nil {
			log.Fatal(err)
		}
		field.AddAllotment(filesystem.Allotment{
			Row:    allotment.Row,
			Col:    allotment.Col,
			Digest: digest.Digest.String(),
		})
	}

	// Merging layers and marshalling filesystem as layer description
	twodfsOpt := llb.WithDescription(map[string]string{"2dfs-field": field.Marshal()})
	return base.WithOutput(llb.Merge(states, twodfsOpt).Output())
}
