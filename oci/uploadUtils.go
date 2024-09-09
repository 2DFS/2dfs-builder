package oci

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func UploadIndex(image OciImageLink, content []byte) error {

	var bearer = ""
	if image.Registry == "docker.io" {
		image.Registry = "registry-1.docker.io"
	}

	// Authenticate only if registryAuth and service are provided
	if image.registryAuth != "" && image.service != "" {
		token, err := getToken(image)
		if err != nil {
			return err
		}

		bearer = "Bearer " + token
	}

	// Get Manifest at https://{registry}/v2/{repository}/manifests/{tag}
	indexRequest, err := http.NewRequest("PUT", fmt.Sprintf("https://%s/v2/%s/manifests/%s", image.Registry, image.Repository, image.Tag), bytes.NewBuffer(content))
	if err != nil {
		return err
	}

	indexRequest.Header.Add("Authorization", bearer)
	indexRequest.Header.Add("Content-Type", v1.MediaTypeImageIndex)
	indexRequest.Header.Set("Content-Length", fmt.Sprint(len(content)))

	//ctx, cancel := context.WithTimeout(indexRequest.Context(), 10*time.Second)
	//defer cancel()

	//indexRequest = indexRequest.WithContext(ctx)

	client := http.DefaultClient
	indexResult, err := client.Do(indexRequest)
	if err != nil {
		return err
	}

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if indexResult.StatusCode == http.StatusUnauthorized || indexResult.StatusCode == 403 && image.registryAuth == "" {
		authHeader := indexResult.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		if len(authHeader) == 0 {
			return fmt.Errorf("error getting index: %d", indexResult.StatusCode)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		if realm == "" || service == "" {
			return fmt.Errorf("error getting index: %d", indexResult.StatusCode)
		}
		image.service = service
		image.registryAuth = realm
		return UploadIndex(image, content)
	}
	if indexResult.StatusCode != http.StatusOK {
		return fmt.Errorf("error getting index: %d", indexResult.StatusCode)
	}
	if err != nil {
		return err
	}

	index, err := ReadIndex(indexResult.Body)
	if index.MediaType != v1.MediaTypeImageIndex {
		return fmt.Errorf("invalid index media type: %s", index.MediaType)
	}

	return nil
}

func UploadBlob(ctx context.Context, image OciImageLink, digest digest.Digest, mediaType string) (io.ReadCloser, error) {
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
	if err != nil {
		return nil, err
	}

	return blobResult.Body, nil
}

func UploadManifest(image OciImageLink, digest string) (io.ReadCloser, error) {

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
