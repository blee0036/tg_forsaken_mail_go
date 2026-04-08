package upload

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadHTML_Success(t *testing.T) {
	expectedUUID := "test-uuid-12345"
	htmlContent := []byte("<html><body>Hello</body></html>")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify URL path and token
		if r.URL.Path != "/upload" {
			t.Errorf("expected path /upload, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "test-token" {
			t.Errorf("expected token test-token, got %s", r.URL.Query().Get("token"))
		}

		// Verify headers
		if r.Header.Get("Content-Type") != "text/html" {
			t.Errorf("expected Content-Type text/html, got %s", r.Header.Get("Content-Type"))
		}

		// Verify body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if string(body) != string(htmlContent) {
			t.Errorf("expected body %q, got %q", string(htmlContent), string(body))
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":    expectedUUID,
			"success": true,
		})
	}))
	defer server.Close()

	uploader := New(server.URL, "test-token")
	uuid, err := uploader.UploadHTML(htmlContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uuid != expectedUUID {
		t.Errorf("expected uuid %q, got %q", expectedUUID, uuid)
	}
}

func TestUploadHTML_SuccessFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":    "some-uuid",
			"success": false,
		})
	}))
	defer server.Close()

	uploader := New(server.URL, "test-token")
	uuid, err := uploader.UploadHTML([]byte("<html></html>"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uuid != "" {
		t.Errorf("expected empty uuid, got %q", uuid)
	}
}

func TestUploadHTML_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	uploader := New(server.URL, "test-token")
	uuid, err := uploader.UploadHTML([]byte("<html></html>"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uuid != "" {
		t.Errorf("expected empty uuid, got %q", uuid)
	}
}

func TestUploadHTML_NetworkError(t *testing.T) {
	// Use a closed server to simulate network error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	uploader := New(server.URL, "test-token")
	uuid, err := uploader.UploadHTML([]byte("<html></html>"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if uuid != "" {
		t.Errorf("expected empty uuid, got %q", uuid)
	}
}

func TestUploadHTML_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	uploader := New(server.URL, "test-token")
	uuid, err := uploader.UploadHTML([]byte("<html></html>"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if uuid != "" {
		t.Errorf("expected empty uuid, got %q", uuid)
	}
}

func TestNew(t *testing.T) {
	uploader := New("https://example.com", "my-token")
	if uploader.uploadURL != "https://example.com" {
		t.Errorf("expected uploadURL https://example.com, got %s", uploader.uploadURL)
	}
	if uploader.uploadToken != "my-token" {
		t.Errorf("expected uploadToken my-token, got %s", uploader.uploadToken)
	}
	if uploader.client == nil {
		t.Error("expected non-nil http client")
	}
}
