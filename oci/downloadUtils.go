package oci

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

var globalToken string = ""

func DeleteToken() {
	globalToken = ""
}

func DownloadIndex(image OciImageLink) (v1.Index, error) {

	var bearer = ""
	if image.Registry == "docker.io" {
		image.Registry = "registry-1.docker.io"
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
	indexRequest, err := http.NewRequest("GET", fmt.Sprintf("%s://%s/v2/%s/manifests/%s", PullPushProtocol, image.Registry, image.Repository, image.Tag), nil)
	if err != nil {
		return v1.Index{}, err
	}

	indexRequest.Header.Add("Authorization", bearer)

	//ctx, cancel := context.WithTimeout(indexRequest.Context(), 10*time.Second)
	//defer cancel()

	//indexRequest = indexRequest.WithContext(ctx)

	client := http.DefaultClient
	indexResult, err := client.Do(indexRequest)
	if err != nil {
		return v1.Index{}, err
	}

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if indexResult.StatusCode == http.StatusUnauthorized || indexResult.StatusCode == 403 && image.registryAuth == "" {
		authHeader := indexResult.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		if len(authHeader) == 0 {
			return v1.Index{}, fmt.Errorf("error getting index: %d", indexResult.StatusCode)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		if realm == "" || service == "" {
			return v1.Index{}, fmt.Errorf("error getting index: %d", indexResult.StatusCode)
		}
		image.service = service
		image.registryAuth = realm
		return DownloadIndex(image)
	}
	if indexResult.StatusCode != http.StatusOK {
		// try getting manifest directly for compatibility with single arch images
		manifestreader, err := DownloadManifest(image, image.Tag)
		if err != nil {
			return v1.Index{}, fmt.Errorf("error getting index: %d", indexResult.StatusCode)
		}
		defer manifestreader.Close()
		manifest, msize, mdigest, err := ReadManifest(manifestreader)
		if err != nil {
			return v1.Index{}, fmt.Errorf("error getting manifest: %d", err)
		}
		//return index with manifest as single element
		parsedDigest, err := digest.Parse(mdigest)
		if err != nil {
			return v1.Index{}, err
		}
		platform := manifest.Config.Platform
		if platform == nil {
			//default platform
			platform = &v1.Platform{
				Architecture: "amd64",
				OS:           "linux",
			}
		}

		idx := v1.Index{
			MediaType: v1.MediaTypeImageIndex,
			Manifests: []v1.Descriptor{
				{
					MediaType: v1.MediaTypeImageManifest,
					Digest:    parsedDigest,
					Size:      msize,
					Platform:  platform,
				},
			},
		}
		idx.SchemaVersion = 2
		return idx, nil
	}

	index, err := ReadIndex(indexResult.Body)
	if index.MediaType != v1.MediaTypeImageIndex {
		return v1.Index{}, fmt.Errorf("invalid index media type: %s", index.MediaType)
	}
	if err != nil {
		return v1.Index{}, err
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

func getToken(image OciImageLink, additionalPermissions ...string) (string, error) {

	if globalToken != "" {
		return globalToken, nil
	}
	permissionString := "pull"
	for _, permission := range additionalPermissions {
		permissionString += "," + permission
	}

	// Get Token at https://{registry}/token\?service\=\{registry}\&scope\="repository:{repository}:pull"
	tokenRequest, err := http.NewRequest("GET", fmt.Sprintf("%s?service=%s&scope=repository:%s:%s", image.registryAuth, image.service, image.Repository, permissionString), nil)
	if err != nil {
		return "", err
	}

	//ctx, cancel := context.WithTimeout(tokenRequest.Context(), 10*time.Second)
	//defer cancel()

	//tokenRequest = tokenRequest.WithContext(ctx)

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

	globalToken = token.Token
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

func DownloadBlob(ctx context.Context, image OciImageLink, digest digest.Digest, mediaType string) (io.ReadCloser, error) {
	var bearer = ""

	if image.Registry == "docker.io" {
		image.Registry = "registry-1.docker.io"
	}

	// Authenticate only if registryAuth and service are provided
	if image.registryAuth != "" && image.service != "" {
		token, err := getToken(image)
		if err != nil {
			return nil, err
		}
		bearer = "Bearer " + token
	}

	// Get Manifest at https://{registry}/v2/{repository}/manifests/{tag}
	blobRequest, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v2/%s/blobs/%s", image.Registry, image.Repository, digest.String()), nil)
	if err != nil {
		return nil, err
	}

	blobRequest.Header.Add("Authorization", bearer)
	blobRequest.Header.Add("Accept", mediaType)

	blobRequest = blobRequest.WithContext(ctx)

	client := http.DefaultClient
	blobResult, err := client.Do(blobRequest)
	if err != nil {
		return nil, err
	}

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if blobResult.StatusCode == http.StatusUnauthorized || blobResult.StatusCode == 403 && image.registryAuth == "" {
		authHeader := blobResult.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		if len(authHeader) == 0 {
			return nil, fmt.Errorf("error getting blob: %d", blobResult.StatusCode)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		image.service = service
		//image.Registry = serice
		image.registryAuth = realm
		if service == "" {
			return nil, fmt.Errorf("error getting blob login service: %d", blobResult.StatusCode)
		}
		return DownloadBlob(ctx, image, digest, mediaType)
	}
	if blobResult.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error getting blob: %d", blobResult.StatusCode)
	}

	return blobResult.Body, nil
}

func DownloadManifest(image OciImageLink, digest string) (io.ReadCloser, error) {

	var bearer = ""
	if image.Registry == "docker.io" {
		image.Registry = "registry-1.docker.io"
	}

	// Authenticate only if registryAuth and service are provided
	if image.registryAuth != "" && image.service != "" {
		token, err := getToken(image)
		if err != nil {
			return nil, err
		}

		bearer = "Bearer " + token
	}

	// Get Manifest at https://{registry}/v2/{repository}/manifests/{tag}
	manifestRequest, err := http.NewRequest("GET", fmt.Sprintf("https://%s/v2/%s/manifests/%s", image.Registry, image.Repository, digest), nil)
	if err != nil {
		return nil, err
	}

	manifestRequest.Header.Add("Authorization", bearer)
	manifestRequest.Header.Add("Accept", v1.MediaTypeImageManifest)

	//ctx, cancel := context.WithTimeout(manifestRequest.Context(), 10*time.Second)
	//defer cancel()

	//manifestRequest = manifestRequest.WithContext(ctx)

	client := http.DefaultClient
	manifestResult, err := client.Do(manifestRequest)

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if manifestResult.StatusCode == http.StatusUnauthorized || manifestResult.StatusCode == 403 && image.registryAuth == "" {
		authHeader := manifestResult.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		if len(authHeader) == 0 {
			return nil, fmt.Errorf("error getting index: %d", manifestResult.StatusCode)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		if realm == "" || service == "" {
			return nil, fmt.Errorf("error getting index: %d", manifestResult.StatusCode)
		}
		image.service = service
		image.registryAuth = realm
		return DownloadManifest(image, digest)
	}
	if manifestResult.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error getting index: %d", manifestResult.StatusCode)
	}
	if err != nil {
		return nil, err
	}

	return manifestResult.Body, nil
}

// ReadManifest returns manifest struct, manifest size, manifest digest
func ReadManifest(manifestReader io.ReadCloser) (v1.Manifest, int64, string, error) {
	buffer := make([]byte, 1024)
	fullread := []byte{}
	for {
		n, err := manifestReader.Read(buffer)
		fullread = append(fullread, buffer[:n]...)
		if err != nil {
			break
		}
	}

	manifestStruct := v1.Manifest{}
	err := json.Unmarshal(fullread, &manifestStruct)
	if err != nil {
		return v1.Manifest{}, 0, "", err
	}
	digest := sha256.Sum256(fullread)
	return manifestStruct, int64(len(fullread)), fmt.Sprintf("sha256:%x", digest), nil
}

func ReadConfig(configReader io.ReadCloser) (v1.Image, error) {
	buffer := make([]byte, 1024)
	fullread := []byte{}
	for {
		n, err := configReader.Read(buffer)
		fullread = append(fullread, buffer[:n]...)
		if err != nil {
			break
		}
	}

	configStruct := v1.Image{}
	err := json.Unmarshal(fullread, &configStruct)
	if err != nil {
		return v1.Image{}, err
	}
	return configStruct, nil
}
