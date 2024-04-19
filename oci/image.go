package oci

import (
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type ContainerImage struct {
	path  string
	index v1.Index
}

func NewContainerImage(path string) (*ContainerImage, error) {
	index, err := LoadIndex(path)
	if err != nil {
		return nil, err
	}
	return &ContainerImage{
		path:  path,
		index: index,
	}, nil
}

func LoadIndex(path string) (v1.Index, error) {
	// if path is an URL use Distribution spec to download image index
	// if path is a local file use fsutil.ReadFile

	// if path is an URL
	return v1.Index{}, nil
}
