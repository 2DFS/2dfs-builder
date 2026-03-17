package main

import (
	"fmt"
	"os"

	"github.com/2DFS/2dfs-builder/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
