package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type gistFile struct {
	Content string `json:"content"`
}

type gistUpdateReq struct {
	Files map[string]gistFile `json:"files"`
}

type gistResp struct {
	Files map[string]struct{
		Filename string `json:"filename"`
		Content  string `json:"content"`
	} `json:"files"`
}

func gistAPIClient() *http.Client { return &http.Client{} }

func gistAuthReq(req *http.Request) {
	token := os.Getenv("GIST_TOKEN")
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
}

func gistReadFile(name string) ([]byte, error) {
	id := os.Getenv("GIST_ID")
	if id == "" { return nil, fmt.Errorf("GIST_ID is empty") }
	url := fmt.Sprintf("https://api.github.com/gists/%s", id)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	gistAuthReq(req)
	resp, err := gistAPIClient().Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return nil, fmt.Errorf("gist read status %d: %s", resp.StatusCode, string(b)) }
	var gr gistResp
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil { return nil, err }
	f, ok := gr.Files[name]
	if !ok { return nil, fmt.Errorf("file %s not found in gist", name) }
	return []byte(f.Content), nil
}

func gistWriteFile(name string, data []byte) error {
	id := os.Getenv("GIST_ID")
	if id == "" { return fmt.Errorf("GIST_ID is empty") }
	url := fmt.Sprintf("https://api.github.com/gists/%s", id)
	payload := gistUpdateReq{Files: map[string]gistFile{name: {Content: string(data)}}}
	buf, _ := json.Marshal(&payload)
	req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(buf))
	gistAuthReq(req)
	resp, err := gistAPIClient().Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode >= 300 { b, _ := io.ReadAll(resp.Body); return fmt.Errorf("gist write status %d: %s", resp.StatusCode, string(b)) }
	return nil
}


