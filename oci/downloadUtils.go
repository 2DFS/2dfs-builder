package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type OciImageLink struct {
	registryAuth string
	Registry     string
	service      string
	Repository   string
	Tag          string
}

type tokenResponse struct {
	Token string `json:"token"`
}

func DownloadIndex(image OciImageLink) (v1.Index, error) {

	var bearer = ""
	if image.Registry == "docker.io" {
		image.Registry = "index.docker.io"
	}

	// Authenticate only if registryAuth and service are provided
	if image.registryAuth != "" && image.service != "" {

		token, err := getToken(image)
		if err != nil {
			return v1.Index{}, err
		}

		bearer = "Bearer " + token

	}

	// Get Manifest at https://{registry}/v2/{repository}/manifests/{tag}
	indexRequest, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v2/%s/manifests/%s", image.Registry, image.Repository, image.Tag), nil)
	if err != nil {
		return v1.Index{}, err
	}

	indexRequest.Header.Add("Authorization", bearer)

	ctx, cancel := context.WithTimeout(indexRequest.Context(), 2*time.Second)
	defer cancel()

	indexRequest = indexRequest.WithContext(ctx)

	client := http.DefaultClient
	indexResult, err := client.Do(indexRequest)

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if indexResult.StatusCode == http.StatusUnauthorized || indexResult.StatusCode == 403 && bearer == "" {
		authHeader := indexResult.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		if len(authHeader) == 0 {
			return v1.Index{}, fmt.Errorf("error getting index: %d", indexResult.StatusCode)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		image.service = service
		image.registryAuth = realm
		return DownloadIndex(image)
	}
	if indexResult.StatusCode != http.StatusOK {
		return v1.Index{}, fmt.Errorf("error getting index: %d", indexResult.StatusCode)
	}
	if err != nil {
		return v1.Index{}, err
	}

	index, err := ReadIndex(indexResult.Body)
	if index.MediaType != v1.MediaTypeImageIndex {
		return v1.Index{}, fmt.Errorf("invalid index media type: %s", index.MediaType)
	}

	return index, nil
}

func ReadIndex(indexReader io.ReadCloser) (v1.Index, error) {
	buffer := make([]byte, 1024)
	fullread := []byte{}
	for {
		n, err := indexReader.Read(buffer)
		fullread = append(fullread, buffer[:n]...)
		if err != nil {
			break
		}
	}

	indexStruct := v1.Index{}
	err := json.Unmarshal(fullread, &indexStruct)
	if err != nil {
		return v1.Index{}, err
	}
	return indexStruct, nil
}

func getToken(image OciImageLink) (string, error) {

	// Get Token at https://{registry}/token\?service\=\{registry}\&scope\="repository:{repository}:pull"
	tokenRequest, err := http.NewRequest("GET", fmt.Sprintf("%s?service=%s&scope=repository:%s:pull", image.registryAuth, image.service, image.Repository), nil)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(tokenRequest.Context(), 2*time.Second)
	defer cancel()

	tokenRequest = tokenRequest.WithContext(ctx)

	client := http.DefaultClient
	tokenResult, err := client.Do(tokenRequest)
	if err != nil {
		return "", err
	}

	if tokenResult.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error getting token: %d", tokenResult.StatusCode)
	}

	responseBuffer := make([]byte, 1024)
	fullResponse := []byte{}
	for {
		n, err := tokenResult.Body.Read(responseBuffer)
		fullResponse = append(fullResponse, responseBuffer[:n]...)
		if err != nil {
			break
		}
	}
	token := tokenResponse{}

	err = json.Unmarshal(fullResponse, &token)
	if err != nil {
		return "", err
	}

	return token.Token, nil
}

// Get the realm and service from the WWW-Authenticate header (realm,service)
func parseWWWAuthenticate(authHeader []string) (string, string) {

	// Split the header into key-value pairs
	pairs := strings.Split(strings.TrimPrefix(authHeader[0], "Bearer "), ",")

	// Parse the key-value pairs
	var service, realm string
	for _, pair := range pairs {
		// Split the pair into key and value
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.Trim(kv[1], "\"")
		switch key {
		case "service":
			service = value
		case "realm":
			realm = value
		}
	}

	return realm, service
}

func DownloadBlob(image OciImageLink, digest digest.Digest) (io.ReadCloser, error) {

	//TODO: Implement this function
	return nil, fmt.Errorf("not implemented")
}
