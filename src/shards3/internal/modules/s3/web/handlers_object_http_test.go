package web

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"shards3/internal/modules/s3/auth"
	"shards3/internal/modules/s3/bucket"
	"shards3/internal/modules/storage/encryption"
	"shards3/internal/modules/storage/interfaces"
	"shards3/internal/modules/storage/metadata"
	"shards3/internal/platform/config"
	"shards3/internal/platform/db"
)

func setupHTTPObjectTest(t *testing.T) http.Handler {
	t.Helper()

	if err := config.LoadConfig(); err != nil {
		t.Fatalf("config.LoadConfig() error: %v", err)
	}

	tempDir := t.TempDir()
	config.Cfg.SQLitePath = filepath.Join(tempDir, "s3_web_test.db")
	config.Cfg.SQLiteBusyTimeoutMS = 5000
	config.Cfg.SQLiteMaxOpenConns = 1
	config.Cfg.KMSPasswordKeyPath = filepath.Join(tempDir, "kms.key")

	database, err := db.New()
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
	})

	if err := encryption.ConfigureKMS(database); err != nil {
		t.Fatalf("encryption.ConfigureKMS() error: %v", err)
	}
	metadata.Configure(database)

	interfaces.SetAvailableBackends(interfaces.RegisterFileTestBackends(3))
	config.Cfg.FailureTolerance = 1

	originalChunkSize := config.Cfg.ChunkSize
	config.Cfg.ChunkSize = 64 * 1024
	t.Cleanup(func() {
		config.Cfg.ChunkSize = originalChunkSize
	})

	if err := os.RemoveAll("./testdata"); err != nil {
		t.Fatalf("cleanup testdata error: %v", err)
	}
	if err := os.MkdirAll("./testdata", 0o755); err != nil {
		t.Fatalf("create testdata error: %v", err)
	}

	s := NewServer(
		bucket.NewService(),
		auth.NewService(auth.Config{
			AccessKeyID:     config.Cfg.S3AccessKeyID,
			SecretAccessKey: config.Cfg.S3SecretAccessKey,
			Region:          config.Cfg.S3Region,
		}),
	)
	return s.Routes()
}

func newBucketRequest(method string, bucketName string, path string, body io.Reader) *http.Request {
	host := bucketName + "." + config.Cfg.FQDN
	url := "http://" + host + path
	req := httptest.NewRequest(method, url, body)
	req.Host = host
	return req
}

func TestHTTPGetObject_FullDownload(t *testing.T) {
	handler := setupHTTPObjectTest(t)

	bucketName := "http-get-bucket"
	key := "/full-object"
	data := bytes.Repeat([]byte("shards3-download-stream-"), 8192)

	putReq := newBucketRequest(http.MethodPut, bucketName, key, bytes.NewReader(data))
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", putRec.Code, putRec.Body.String())
	}

	getReq := newBucketRequest(http.MethodGet, bucketName, key, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	if !bytes.Equal(getRec.Body.Bytes(), data) {
		t.Fatal("GET body does not match uploaded object data")
	}

	contentLength := getRec.Header().Get("Content-Length")
	if contentLength != strconv.Itoa(len(data)) {
		t.Fatalf("Content-Length = %q, want %d", contentLength, len(data))
	}
}

func TestHTTPGetObject_RangeDownload(t *testing.T) {
	handler := setupHTTPObjectTest(t)

	bucketName := "http-range-bucket"
	key := "/range-object"
	data := bytes.Repeat([]byte("0123456789abcdef"), 4096)

	putReq := newBucketRequest(http.MethodPut, bucketName, key, bytes.NewReader(data))
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", putRec.Code, putRec.Body.String())
	}

	start := 17
	inclusiveEnd := 2048
	getReq := newBucketRequest(http.MethodGet, bucketName, key, nil)
	getReq.Header.Set("Range", "bytes="+strconv.Itoa(start)+"-"+strconv.Itoa(inclusiveEnd))

	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusPartialContent {
		t.Fatalf("GET status = %d, want %d, body = %s", getRec.Code, http.StatusPartialContent, getRec.Body.String())
	}

	expected := data[start : inclusiveEnd+1]
	if !bytes.Equal(getRec.Body.Bytes(), expected) {
		t.Fatal("range GET body does not match expected slice")
	}

	contentLength := getRec.Header().Get("Content-Length")
	if contentLength != strconv.Itoa(len(expected)) {
		t.Fatalf("Content-Length = %q, want %d", contentLength, len(expected))
	}

	contentRange := getRec.Header().Get("Content-Range")
	expectedContentRange := "bytes " + strconv.Itoa(start) + "-" + strconv.Itoa(inclusiveEnd) + "/" + strconv.Itoa(len(data))
	if contentRange != expectedContentRange {
		t.Fatalf("Content-Range = %q, want %q", contentRange, expectedContentRange)
	}
}

func TestHTTPGetObject_InvalidRange(t *testing.T) {
	handler := setupHTTPObjectTest(t)

	bucketName := "http-invalid-range-bucket"
	key := "/invalid-range-object"
	data := []byte("0123456789")

	putReq := newBucketRequest(http.MethodPut, bucketName, key, bytes.NewReader(data))
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", putRec.Code, putRec.Body.String())
	}

	getReq := newBucketRequest(http.MethodGet, bucketName, key, nil)
	getReq.Header.Set("Range", "bytes=20-30")

	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("GET status = %d, want %d", getRec.Code, http.StatusRequestedRangeNotSatisfiable)
	}

	var s3Err errorResponse
	if err := xml.Unmarshal(getRec.Body.Bytes(), &s3Err); err != nil {
		t.Fatalf("failed to decode error XML: %v", err)
	}
	if s3Err.Code != "InvalidRange" {
		t.Fatalf("error code = %q, want %q", s3Err.Code, "InvalidRange")
	}
}
