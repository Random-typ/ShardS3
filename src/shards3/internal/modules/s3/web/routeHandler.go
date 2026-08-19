package web

import (
	"net/http"
)

func hasKey(r *http.Request) bool {
	return r.URL.Path != "/" && r.URL.Path != ""
}

func (s *Server) handleBucketsGet(w http.ResponseWriter, r *http.Request) {
	if hasKey(r) {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "request cannot be handled", "")
		return
	}
	s.ListBuckets(w, r)
}

func (s *Server) handleBucketPresentGet(w http.ResponseWriter, r *http.Request) {
	if hasKey(r) {
		switch {
		case r.URL.Query().Has("tagging"):
			s.GetObjectTagging(w, r)
			return
		case r.URL.Query().Has("acl"):
			s.GetObjectAcl(w, r)
			return
		case r.URL.Query().Has("uploadId"):
			s.ListParts(w, r)
			return
		default:
			s.GetObject(w, r)
			return
		}
	}
	switch {
	case r.URL.Query().Has("location"):
		s.GetBucketLocation(w, r)
		return
	case r.URL.Query().Has("tagging"):
		s.GetBucketTagging(w, r)
		return
	case r.URL.Query().Has("acl"):
		s.GetBucketAcl(w, r)
		return
	case r.URL.Query().Has("encryption"):
		s.GetBucketEncryption(w, r)
		return
	case r.URL.Query().Has("acl"):
		s.GetBucketAcl(w, r)
		return
	case r.URL.Query().Has("uploads"):
		s.ListMultipartUploads(w, r)
		return
	default:
		s.ListObjectsV2(w, r)
	}
}

func (s *Server) handleBucketPresentHead(w http.ResponseWriter, r *http.Request) {
	if hasKey(r) {
		s.HeadObject(w, r)
		return
	}
	s.HeadBucket(w, r)
}

func (s *Server) handleBucketPresentPut(w http.ResponseWriter, r *http.Request) {
	if !hasKey(r) {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "request cannot be handled", "")
		return
	}
	switch {
	case r.URL.Query().Has("uploadId"):
		s.UploadPart(w, r)
		return
	default:
		s.PutObject(w, r)
		return
	}
}

func (s *Server) handleBucketPresentPost(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Query().Has("delete"):
		s.DeleteObjects(w, r)
		return
	case r.URL.Query().Has("uploads"):
		s.CreateMultipartUpload(w, r)
		return
	case r.URL.Query().Has("uploadId"):
		s.CompleteMultipartUpload(w, r)
		return
	default:
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "request cannot be handled", "")
	}
}

func (s *Server) handleBucketPresentDelete(w http.ResponseWriter, r *http.Request) {
	if !hasKey(r) {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "request cannot be handled", "")
		return
	}
	switch {
	case r.URL.Query().Has("uploadId"):
		s.AbortMultipartUpload(w, r)
		return
	default:
		s.DeleteObject(w, r)
		return
	}
}
