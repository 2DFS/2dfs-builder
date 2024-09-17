package oci

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func UploadManifest(link OciImageLink, indexBytes []byte, mediaType string, token string) (string, error) {

	// Authenticate only if registryAuth and service are provided
	bearer := ""
	if link.registryAuth != "" && link.service != "" && token == "" {
		token, err := getToken(link, "push")
		if err != nil {
			return "", err
		}

		bearer = "Bearer " + token
	}

	url := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", PullPushProtocol, link.Registry, link.Repository, link.Reference)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(indexBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mediaType)
	req.Header.Add("Authorization", bearer)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == 403 && link.registryAuth == "" {
		authHeader := response.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		fmt.Println(response.Header)
		if len(authHeader) == 0 {
			return "", fmt.Errorf("error pushing index: %d, with auth header:%v", response.StatusCode, authHeader)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		if realm == "" || service == "" {
			return "", fmt.Errorf("error pushing index: %d", response.StatusCode)
		}
		link.service = service
		link.registryAuth = realm
		return UploadManifest(link, indexBytes, mediaType, "")
	}

	log.Println("Index Upload Status: ", response.Status)
	return token, nil

}

func UploadBlob(link OciImageLink, blobReader io.ReadCloser, contentLength int, token string) (string, error) {

	// Authenticate only if registryAuth and service are provided
	bearer := ""
	if link.registryAuth != "" && link.service != "" && token == "" {
		token, err := getToken(link, "push")
		if err != nil {
			return "", err
		}

		bearer = "Bearer " + token
	}

	// PUT + POST monolithical approach

	// --- STEP 1: PUT-REQ, obtain a session ID
	url := fmt.Sprintf("%s://%s/v2/%s/blobs/uploads/?digest=%s", PullPushProtocol, link.Registry, link.Repository, link.Reference)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", bearer)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	// If the request is unauthorized, try to get a token and retry
	// This works only if bearer was empty, thus auth was not attempted
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == 403 && link.registryAuth == "" {
		authHeader := response.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
		fmt.Println(response.Header)
		if len(authHeader) == 0 {
			return "", fmt.Errorf("error pushing index: %d, with auth header:%v", response.StatusCode, authHeader)
		}
		realm, service := parseWWWAuthenticate(authHeader)
		if realm == "" || service == "" {
			return "", fmt.Errorf("error pushing blob [POST]: %d", response.StatusCode)
		}
		link.service = service
		link.registryAuth = realm
		return UploadBlob(link, blobReader, contentLength, "")
	}

	location := ""
	if response.StatusCode == http.StatusAccepted {
		// The response body contains the session ID
		// get Location
		location = response.Header.Get("Location")
	} else {
		return token, fmt.Errorf("error pushing blob [POST]: %d", response.StatusCode)
	}

	// --- STEP 2: POST-REQ, use sessiond ID to perform upload
	if strings.Contains(location, "?") {
		url = fmt.Sprintf("%s&digest=sha256:%s", location, link.Reference)
	} else {
		url = fmt.Sprintf("%s?digest=sha256:%s", location, link.Reference)
	}

	req, err = http.NewRequest("PUT", url, blobReader)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", contentLength))
	req.Header.Add("Authorization", bearer)
	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}

	response, err = client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		return token, fmt.Errorf("error pushing blob [PUT]: %d", response.StatusCode)
	}

	return token, nil
}

type Base64Reader struct {
	reader       io.ReadCloser
	encoder      *io.WriteCloser
	resultReader io.ReadCloser
}

func NewBase64Reader(reader io.ReadCloser) *Base64Reader {
	resultReader, resultWriter := io.Pipe()
	encoder := base64.NewEncoder(base64.StdEncoding, resultWriter)
	return &Base64Reader{
		reader:       reader,
		encoder:      &encoder,
		resultReader: resultReader,
	}
}

func (r *Base64Reader) Read(p []byte) (int, error) {
	tmpBuffer := make([]byte, len(p))
	n, err := r.reader.Read(tmpBuffer)
	if err != nil {
		return n, err
	}

	go func() {
		_, _ = (*r.encoder).Write(tmpBuffer[:n])
	}()

	return r.resultReader.Read(p)
}

func (r *Base64Reader) Close() error {
	_ = r.reader.Close()
	_ = (*r.encoder).Close()
	return r.resultReader.Close()
}

func reverseBase64Reader(reader io.Reader) io.Reader {
	input, err := io.ReadAll(reader)
	if err != nil {
		log.Fatal(err)
	}
	output := base64.StdEncoding.EncodeToString(input)
	//return reader from output
	return bufio.NewReader(bytes.NewReader([]byte(output)))
}
