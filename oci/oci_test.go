package oci

import (
	"strings"
	"testing"
)

func TestToken(t *testing.T) {

	link := OciImageLink{
		registryAuth: "https://auth.docker.io/token",
		Registry:     "index.docker.io",
		service:      "registry.docker.io",
		Repository:   "library/ubuntu",
		Tag:          "latest",
	}

	_, err := getToken(link)

	if err != nil {
		t.Errorf("DownloadIndex error: %v", err)
	}

}

func TestDownloadIndexDocker(t *testing.T) {

	link := OciImageLink{
		Registry:   "index.docker.io",
		Repository: "library/nginx",
		Tag:        "latest",
	}

	index, err := DownloadIndex(link)

	if err != nil {
		t.Errorf("DownloadIndex error: %v", err)
		t.Failed()
	}

	if index.SchemaVersion != 2 {
		t.Errorf("Invalid Schems error, expected %d, given %d", 2, index.SchemaVersion)
		t.Failed()
	}

	if strings.Contains(index.Manifests[0].Annotations["org.opencontainers.image.src"], "nginx") {
		t.Errorf("not a nginx image")
		t.Failed()
	}

}

func TestDownloadIndexGhcrFail(t *testing.T) {

	link := OciImageLink{
		Registry:   "ghcr.io",
		Repository: "giobart/message-broker/message-broker",
		Tag:        "v1.2.4",
	}

	_, err := DownloadIndex(link)

	if err != nil {
		if !strings.Contains(err.Error(), "invalid index media type") {
			t.Errorf("DownloadIndex error: %v", err)
			t.Fatal()
		}
	}

}

func TestParseWWWAuth(t *testing.T) {

	// Example: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"
	auth := `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"`

	realm, service := parseWWWAuthenticate([]string{auth})

	if realm != "https://auth.docker.io/token" {
		t.Errorf("Invalid realm error, expected %s, given %s", "https://auth.docker.io/token", realm)
	}

	if service != "registry.docker.io" {
		t.Errorf("Invalid service error, expected %s, given %s", "registry.docker.io", service)
	}

}
