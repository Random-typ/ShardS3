package web

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"shards3/services/shards3/internal/modules/s3/auth"
	"shards3/services/shards3/internal/modules/s3/bucket"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/modules/storage/objectManager"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
)

func setupHTTPMultipartTest(t *testing.T) *Server {
	t.Helper()

	if err := config.LoadConfig(); err != nil {
		t.Fatalf("config.LoadConfig() error: %v", err)
	}

	tempDir := t.TempDir()
	config.Cfg.SQLitePath = filepath.Join(tempDir, "s3_multipart_web_test.db")
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

	return NewServer(
		bucket.NewService(),
		auth.NewService(auth.Config{
			AccessKeyID:     config.Cfg.S3AccessKeyID,
			SecretAccessKey: config.Cfg.S3SecretAccessKey,
			Region:          config.Cfg.S3Region,
		}),
	)
}

func newMultipartRequest(method string, bucketName string, pathWithQuery string, body io.Reader) *http.Request {
	host := bucketName + "." + config.Cfg.FQDN
	url := "http://" + host + pathWithQuery
	req := httptest.NewRequest(method, url, body)
	req.Host = host
	return req
}

func TestHTTPMultipart_CreateAndListUploads(t *testing.T) {
	s := setupHTTPMultipartTest(t)

	bucketName := "http-multipart-bucket"
	key := "/multipart-object"

	createReq := newMultipartRequest(http.MethodPost, bucketName, key+"?uploads", nil)
	createRec := httptest.NewRecorder()
	s.CreateMultipartUpload(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateMultipartUpload status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	var initiated InitiateMultipartUploadResult
	if err := xml.Unmarshal(createRec.Body.Bytes(), &initiated); err != nil {
		t.Fatalf("failed to decode initiate response XML: %v", err)
	}
	if initiated.Bucket != bucketName {
		t.Fatalf("initiated bucket = %q, want %q", initiated.Bucket, bucketName)
	}
	if initiated.Key != key {
		t.Fatalf("initiated key = %q, want %q", initiated.Key, key)
	}
	if initiated.UploadID == "" {
		t.Fatal("expected non-empty UploadID")
	}

	listReq := newMultipartRequest(http.MethodGet, bucketName, key, nil)
	listRec := httptest.NewRecorder()
	s.ListMultipartUploads(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("ListMultipartUploads status = %d, body = %s", listRec.Code, listRec.Body.String())
	}

	var listed ListMultipartUploadsResult
	if err := xml.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to decode list uploads XML: %v", err)
	}
	if listed.Bucket != bucketName {
		t.Fatalf("listed bucket = %q, want %q", listed.Bucket, bucketName)
	}
	if len(listed.Uploads) != 1 {
		t.Fatalf("uploads len = %d, want 1", len(listed.Uploads))
	}
	if listed.Uploads[0].UploadID != initiated.UploadID {
		t.Fatalf("listed upload id = %q, want %q", listed.Uploads[0].UploadID, initiated.UploadID)
	}
}

func TestHTTPMultipart_ListParts(t *testing.T) {
	s := setupHTTPMultipartTest(t)

	bucketName := "http-multipart-parts-bucket"
	key := "/parts-object"

	createReq := newMultipartRequest(http.MethodPost, bucketName, key+"?uploads", nil)
	createRec := httptest.NewRecorder()
	s.CreateMultipartUpload(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("CreateMultipartUpload status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	var initiated InitiateMultipartUploadResult
	if err := xml.Unmarshal(createRec.Body.Bytes(), &initiated); err != nil {
		t.Fatalf("failed to decode initiate response XML: %v", err)
	}

	part1Data := bytes.Repeat([]byte("part-one-data-"), 2048)
	part2Data := bytes.Repeat([]byte("part-two-data-"), 3072)

	putPart1Req := newMultipartRequest(http.MethodPut, bucketName, key+"?partNumber=1&uploadId="+initiated.UploadID, bytes.NewReader(part1Data))
	putPart1Rec := httptest.NewRecorder()
	s.UploadPart(putPart1Rec, putPart1Req)
	if putPart1Rec.Code != http.StatusOK {
		t.Fatalf("UploadPart(part1) status = %d, body = %s", putPart1Rec.Code, putPart1Rec.Body.String())
	}

	putPart2Req := newMultipartRequest(http.MethodPut, bucketName, key+"?partNumber=2&uploadId="+initiated.UploadID, bytes.NewReader(part2Data))
	putPart2Rec := httptest.NewRecorder()
	s.UploadPart(putPart2Rec, putPart2Req)
	if putPart2Rec.Code != http.StatusOK {
		t.Fatalf("UploadPart(part2) status = %d, body = %s", putPart2Rec.Code, putPart2Rec.Body.String())
	}

	req := newMultipartRequest(http.MethodGet, bucketName, key+"?uploadId="+initiated.UploadID+"&max-parts=1000", nil)
	rec := httptest.NewRecorder()
	s.ListParts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListParts status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var listed ListPartsResult
	if err := xml.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("failed to decode list parts XML: %v", err)
	}
	if listed.UploadID != initiated.UploadID {
		t.Fatalf("list UploadID = %q, want %q", listed.UploadID, initiated.UploadID)
	}
	if len(listed.Parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(listed.Parts))
	}

	if listed.Parts[0].PartNumber != 1 {
		t.Fatalf("part[0].PartNumber = %d, want 1", listed.Parts[0].PartNumber)
	}
	part1Etag := putPart1Rec.Header().Get("ETag")
	if part1Etag == "" {
		t.Fatal("UploadPart(part1) missing ETag header")
	}
	if listed.Parts[0].ETag != part1Etag {
		t.Fatalf("part[0].ETag = %q, want %q", listed.Parts[0].ETag, part1Etag)
	}

	if listed.Parts[1].PartNumber != 2 {
		t.Fatalf("part[1].PartNumber = %d, want 2", listed.Parts[1].PartNumber)
	}
	part2Etag := putPart2Rec.Header().Get("ETag")
	if part2Etag == "" {
		t.Fatal("UploadPart(part2) missing ETag header")
	}
	if listed.Parts[1].ETag != part2Etag {
		t.Fatalf("part[1].ETag = %q, want %q", listed.Parts[1].ETag, part2Etag)
	}
}

func TestHTTPMultipart_AbortUpload(t *testing.T) {
	s := setupHTTPMultipartTest(t)

	location := object.ObjectLocation{Bucket: object.Bucket{Name: "http-multipart-abort-bucket"}, Key: "/abort-object"}
	upload, err := objectManager.CreateMultipartUpload(location)
	if err != nil {
		t.Fatalf("CreateMultipartUpload() setup error: %v", err)
	}

	req := newMultipartRequest(http.MethodDelete, location.Bucket.Name, location.Key+"?uploadId="+upload.UploadID, nil)
	rec := httptest.NewRecorder()
	s.AbortMultipartUpload(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("AbortMultipartUpload status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := metadata.GetMultipartUpload(upload.UploadID); err == nil {
		t.Fatal("expected upload to be removed after abort")
	}
}
