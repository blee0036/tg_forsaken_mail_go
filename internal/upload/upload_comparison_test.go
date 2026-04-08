package upload

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Task 7.2: 对比验证：HTML 上传模块
// Validates: Requirement 5.7
// Verifies Go version sends HTTP requests with format (URL, Headers, Body)
// matching Node version axios requests, and return values match Node behavior.

// --- Comparison 1: Request URL format matches Node axios ---
// Node: url: upload_url + "/upload?token=" + upload_token
func TestComparison_RequestURLFormat(t *testing.T) {
	var capturedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"uuid": "abc", "success": true})
	}))
	defer server.Close()

	token := "my-secret-token"
	uploader := New(server.URL, token)
	uploader.UploadHTML([]byte("<html></html>"))

	// Node axios builds: {upload_url}/upload?token={upload_token}
	// The path received by the server should be /upload?token=my-secret-token
	expected := "/upload?token=" + token
	if capturedURL != expected {
		t.Errorf("URL mismatch with Node axios format\n  Go sent:  %q\n  Expected: %q (Node: upload_url + \"/upload?token=\" + upload_token)", capturedURL, expected)
	}
}

// --- Comparison 2: Request method is POST ---
// Node: method: 'post'
func TestComparison_RequestMethodPOST(t *testing.T) {
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"uuid": "abc", "success": true})
	}))
	defer server.Close()

	uploader := New(server.URL, "token")
	uploader.UploadHTML([]byte("<html></html>"))

	if capturedMethod != http.MethodPost {
		t.Errorf("method mismatch: Go sent %q, Node axios uses 'post'", capturedMethod)
	}
}

// --- Comparison 3: Request headers match Node axios config ---
// Node: headers: { 'Content-Type': 'text/html', 'Content-Length': htmlBuffer.length }
func TestComparison_RequestHeaders(t *testing.T) {
	htmlContent := []byte("<html><body><h1>Test Email</h1></body></html>")
	var capturedContentType string
	var capturedContentLength string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		capturedContentLength = r.Header.Get("Content-Length")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"uuid": "abc", "success": true})
	}))
	defer server.Close()

	uploader := New(server.URL, "token")
	uploader.UploadHTML(htmlContent)

	// Node sets Content-Type: 'text/html'
	if capturedContentType != "text/html" {
		t.Errorf("Content-Type mismatch: Go sent %q, Node axios sends 'text/html'", capturedContentType)
	}

	// Node sets Content-Length: htmlBuffer.length (integer as string in header)
	expectedLen := strconv.Itoa(len(htmlContent))
	if capturedContentLength != expectedLen {
		t.Errorf("Content-Length mismatch: Go sent %q, Node axios sends %q", capturedContentLength, expectedLen)
	}
}

// --- Comparison 4: Request body is raw HTML bytes ---
// Node: data: htmlBuffer (Buffer.from(data.html, "utf-8"))
func TestComparison_RequestBodyRawHTML(t *testing.T) {
	htmlContent := []byte("<html><body>邮件内容 with unicode 🎉</body></html>")
	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"uuid": "abc", "success": true})
	}))
	defer server.Close()

	uploader := New(server.URL, "token")
	uploader.UploadHTML(htmlContent)

	if string(capturedBody) != string(htmlContent) {
		t.Errorf("body mismatch:\n  Go sent:  %q\n  Expected: %q (raw HTML bytes like Node axios data: htmlBuffer)", string(capturedBody), string(htmlContent))
	}
}

// --- Comparison 5: Success response (200 + success:true) → returns uuid ---
// Node: if (success) { return uuid; }
func TestComparison_SuccessReturnsUUID(t *testing.T) {
	expectedUUID := "550e8400-e29b-41d4-a716-446655440000"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":    expectedUUID,
			"success": true,
		})
	}))
	defer server.Close()

	uploader := New(server.URL, "token")
	uuid, err := uploader.UploadHTML([]byte("<html>test</html>"))

	// Node returns uuid string on success
	if err != nil {
		t.Fatalf("unexpected error: %v (Node would not throw)", err)
	}
	if uuid != expectedUUID {
		t.Errorf("uuid mismatch: Go returned %q, expected %q (Node returns uuid from response)", uuid, expectedUUID)
	}
}

// --- Comparison 6: Failure response (200 + success:false) → returns empty string ---
// Node: console.error("Data upload failed."); return null;
// Go equivalent of null: empty string ""
func TestComparison_FailureReturnsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":    "some-uuid",
			"success": false,
		})
	}))
	defer server.Close()

	uploader := New(server.URL, "token")
	uuid, err := uploader.UploadHTML([]byte("<html>test</html>"))

	// Node returns null when success is false; Go returns "" (empty string)
	if err != nil {
		t.Fatalf("unexpected error: %v (Node does not throw on success:false)", err)
	}
	if uuid != "" {
		t.Errorf("Go returned %q, expected empty string (Node returns null when success is false)", uuid)
	}
}

// --- Comparison 7: Non-200 response → returns empty string ---
// Node: console.error(`Data upload failed with status code: ${response.status}`); return null;
// Go equivalent of null: empty string ""
func TestComparison_Non200ReturnsEmpty(t *testing.T) {
	statusCodes := []int{
		http.StatusBadRequest,          // 400
		http.StatusUnauthorized,        // 401
		http.StatusForbidden,           // 403
		http.StatusNotFound,            // 404
		http.StatusInternalServerError, // 500
		http.StatusServiceUnavailable,  // 503
	}

	for _, code := range statusCodes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				w.Write([]byte("error"))
			}))
			defer server.Close()

			uploader := New(server.URL, "token")
			uuid, err := uploader.UploadHTML([]byte("<html>test</html>"))

			// Node returns null for non-200 status; Go returns ""
			if err != nil {
				t.Fatalf("status %d: unexpected error: %v (Node does not throw on non-200)", code, err)
			}
			if uuid != "" {
				t.Errorf("status %d: Go returned %q, expected empty string (Node returns null)", code, uuid)
			}
		})
	}
}

// --- Comparison 8: Network error → returns empty string with error ---
// Node: catch (error) { console.error(`Data upload failed error : ${error.stack}`); return null; }
// Go: returns ("", error)
func TestComparison_NetworkErrorReturnsEmptyWithError(t *testing.T) {
	// Create and immediately close server to simulate network error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	uploader := New(server.URL, "token")
	uuid, err := uploader.UploadHTML([]byte("<html>test</html>"))

	// Node returns null on network error; Go returns "" with non-nil error
	if err == nil {
		t.Fatal("expected error for network failure (Node catches error in try/catch)")
	}
	if uuid != "" {
		t.Errorf("Go returned %q, expected empty string (Node returns null on network error)", uuid)
	}
}

// --- Comparison: Full request format matches Node axios.request(post_config) ---
// Validates all aspects of the request in a single integrated test
func TestComparison_FullRequestFormatMatchesNodeAxios(t *testing.T) {
	htmlContent := []byte("<html><body><p>Full integration test content</p></body></html>")
	uploadToken := "test-upload-token-123"

	var (
		capturedMethod        string
		capturedPath          string
		capturedQuery         string
		capturedContentType   string
		capturedContentLength string
		capturedBody          []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedContentType = r.Header.Get("Content-Type")
		capturedContentLength = r.Header.Get("Content-Length")
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":    "integration-uuid",
			"success": true,
		})
	}))
	defer server.Close()

	uploader := New(server.URL, uploadToken)
	uuid, err := uploader.UploadHTML(htmlContent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Node axios post_config:
	//   method: 'post'
	//   url: upload_url + "/upload?token=" + upload_token
	//   headers: { 'Content-Type': 'text/html', 'Content-Length': htmlBuffer.length }
	//   data: htmlBuffer

	if capturedMethod != "POST" {
		t.Errorf("method: Go=%q, Node='post'", capturedMethod)
	}
	if capturedPath != "/upload" {
		t.Errorf("path: Go=%q, Node='/upload'", capturedPath)
	}
	expectedQuery := "token=" + uploadToken
	if capturedQuery != expectedQuery {
		t.Errorf("query: Go=%q, Node=%q", capturedQuery, expectedQuery)
	}
	if capturedContentType != "text/html" {
		t.Errorf("Content-Type: Go=%q, Node='text/html'", capturedContentType)
	}
	expectedLen := strconv.Itoa(len(htmlContent))
	if capturedContentLength != expectedLen {
		t.Errorf("Content-Length: Go=%q, Node=%q", capturedContentLength, expectedLen)
	}
	if string(capturedBody) != string(htmlContent) {
		t.Errorf("body mismatch: Go sent %d bytes, expected %d bytes", len(capturedBody), len(htmlContent))
	}
	if uuid != "integration-uuid" {
		t.Errorf("uuid: Go=%q, expected 'integration-uuid'", uuid)
	}
}
