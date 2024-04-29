package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

	token, err := getToken(image)
	if err != nil {
		return v1.Index{}, err
	}

	var bearer = "Bearer " + token

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
	if err != nil {
		return v1.Index{}, err
	}

	if indexResult.StatusCode != http.StatusOK {
		return v1.Index{}, fmt.Errorf("error getting index: %d", indexResult.StatusCode)
	}

	responseBuffer := make([]byte, 1024)
	fullResponse := []byte{}
	for {
		n, err := indexResult.Body.Read(responseBuffer)
		fullResponse = append(fullResponse, responseBuffer[:n]...)
		if err != nil {
			break
		}
	}
	index := v1.Index{}

	err = json.Unmarshal(fullResponse, &index)
	if err != nil {
		return v1.Index{}, err
	}

	return index, nil
}

func getToken(image OciImageLink) (string, error) {

	// Get Token at https://{registry}/token\?service\=\{registry}\&scope\="repository:{repository}:pull"
	tokenRequest, err := http.NewRequest("GET", fmt.Sprintf("https://%s/token?service=%s&scope=repository:%s:pull", image.registryAuth, image.service, image.Repository), nil)
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
