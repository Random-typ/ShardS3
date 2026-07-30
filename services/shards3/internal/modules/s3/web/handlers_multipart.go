package web

import (
	"net/http"
)

func (s *Server) CreateMultipartUpload(w http.ResponseWriter, r *http.Request) {
	//bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	//if !ok {
	//	writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket name not found in host", r.URL.Path)
	//	return
	//}
}

func (s *Server) UploadPart(w http.ResponseWriter, r *http.Request) {
}

func (s *Server) CompleteMultipartUpload(w http.ResponseWriter, r *http.Request) {
}

func (s *Server) AbortMultipartUpload(w http.ResponseWriter, r *http.Request) {
}

func (s *Server) ListMultipartUploads(w http.ResponseWriter, r *http.Request) {
}

func (s *Server) ListParts(w http.ResponseWriter, r *http.Request) {
}
