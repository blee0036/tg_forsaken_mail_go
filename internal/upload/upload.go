package upload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

// Uploader handles HTML email uploads to Cloudflare Worker.
type Uploader struct {
	uploadURL   string
	uploadToken string
	client      *http.Client
}

// uploadResponse represents the JSON response from the upload endpoint.
type uploadResponse struct {
	UUID    string `json:"uuid"`
	Success bool   `json:"success"`
}

// New creates a new Uploader instance.
func New(uploadURL, uploadToken string) *Uploader {
	return &Uploader{
		uploadURL:   uploadURL,
		uploadToken: uploadToken,
		client:      &http.Client{},
	}
}

// UploadHTML uploads HTML content to the Cloudflare Worker and returns the uuid.
// On failure, it logs the error and returns an empty string.
func (u *Uploader) UploadHTML(htmlContent []byte) (string, error) {
	url := fmt.Sprintf("%s/upload?token=%s", u.uploadURL, u.uploadToken)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(htmlContent))
	if err != nil {
		log.Printf("Data upload failed error : %v", err)
		return "", err
	}

	req.Header.Set("Content-Type", "text/html")
	req.Header.Set("Content-Length", strconv.Itoa(len(htmlContent)))
	req.ContentLength = int64(len(htmlContent))

	resp, err := u.client.Do(req)
	if err != nil {
		log.Printf("Data upload failed error : %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var result uploadResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			log.Printf("Data upload failed error : %v", err)
			return "", err
		}

		if result.Success {
			return result.UUID, nil
		}

		log.Println("Data upload failed.")
		return "", nil
	}

	log.Printf("Data upload failed with status code: %d", resp.StatusCode)
	return "", nil
}
