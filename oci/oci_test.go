package oci

import (
	"strings"
	"testing"
)

func TestToken(t *testing.T) {

	link := OciImageLink{
		registryAuth: "auth.docker.io",
		Registry:     "index.docker.io",
		service:      "registry.docker.io",
		realm:        "https://auth.docker.io/token",
		Repository:   "library/ubuntu",
		Tag:          "latest",
	}

	_, err := getToken(link)

	if err != nil {
		t.Errorf("DownloadIndex error: %v", err)
	}

}

func TestDownloadIndex(t *testing.T) {

	link := OciImageLink{
		Registry:     "index.docker.io",
		Repository:   "library/nginx",
		Tag:          "latest",
		service:      "registry.docker.io",
		registryAuth: "auth.docker.io",
	}

	index, err := DownloadIndex(link)

	if err != nil {
		t.Errorf("DownloadIndex error: %v", err)
	}

	if index.SchemaVersion != 2 {
		t.Errorf("Invalid Schems error, expected %d, given %d", 2, index.SchemaVersion)
	}

	if strings.Contains(index.Manifests[0].Annotations["org.opencontainers.image.src"], "nginx") {
		t.Errorf("not a nginx image")
	}

}
