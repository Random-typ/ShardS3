package web

import (
	"encoding/xml"
	"errors"
	"log"
	"net/http"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/modules/storage/objectManager"
	"shards3/services/shards3/internal/platform/config"
	"strconv"
	"strings"
	"time"
)

type ListBucketResult struct {
	XMLName     xml.Name `xml:"ListBucketResult"`
	Xmlns       string   `xml:"xmlns,attr"`
	IsTruncated bool     `xml:"IsTruncated,omitempty"`

	Contents []Contents `xml:"Contents,omitempty"`

	Name                  string         `xml:"Name,omitempty"`
	Prefix                string         `xml:"Prefix,omitempty"`
	Delimiter             string         `xml:"Delimiter,omitempty"`
	MaxKeys               int            `xml:"MaxKeys,omitempty"`
	CommonPrefixes        []CommonPrefix `xml:"CommonPrefixes,omitempty"`
	EncodingType          string         `xml:"EncodingType,omitempty"`
	KeyCount              int            `xml:"KeyCount,omitempty"`
	ContinuationToken     string         `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string         `xml:"NextContinuationToken,omitempty"`
	StartAfter            string         `xml:"StartAfter,omitempty"`
}

type CommonPrefix struct {
	Prefix string `xml:"Prefix,omitempty"`
}

type Contents struct {
	ChecksumAlgorithm   string `xml:"ChecksumAlgorithm,omitempty"`
	ChecksumType        string `xml:"ChecksumType,omitempty"`
	ETag                uint64 `xml:"ETag,omitempty"`
	Key                 string `xml:"Key,omitempty"`
	LastModified        string `xml:"LastModified,omitempty"`
	Owner               Owner  `xml:"Owner,omitempty"`
	IsRestoreInProgress bool   `xml:"RestoreStatus>IsRestoreInProgress,omitempty"`
	RestoreExpiryDate   string `xml:"RestoreStatus>RestoreExpiryDate,omitempty"`
	Size                int64  `xml:"Size,omitempty"`
	StorageClass        string `xml:"StorageClass,omitempty"`
}

type Object struct {
	ETag             uint64 `xml:"ETag,omitempty"`
	Key              string `xml:"Key,omitempty"`
	LastModifiedTime int64  `xml:"LastModifiedTime,omitempty"`
	Size             int64  `xml:"Size,omitempty"`
	VersionId        string `xml:"VersionId,omitempty"`
}

type Delete struct {
	XMLName xml.Name `xml:"Delete"`

	Objects []Object `xml:"Object,omitempty"`
	Quiet   bool     `xml:"Quiet,omitempty"`
}

type DeletedObject struct {
	DeleteMarker          bool   `xml:"DeleteMarker,omitempty"`
	DeleteMarkerVersionId string `xml:"DeleteMarkerVersionId,omitempty"`
	Key                   string `xml:"Key,omitempty"`
	VersionId             string `xml:"VersionId,omitempty"`
}

type DeleteError struct {
	Code      string `xml:"Code,omitempty"`
	Key       string `xml:"Key,omitempty"`
	Message   string `xml:"Message,omitempty"`
	VersionId string `xml:"VersionId,omitempty"`
}

type DeleteResult struct {
	XMLName xml.Name `xml:"DeleteResult"`

	Deleted []DeletedObject `xml:"Deleted,omitempty"`
	Errors  []DeleteError   `xml:"Error,omitempty"`
}

// Checks wether the If-Match header matches the object's ETag. If it does, returns false, otherwise true.
// Writes S3 error on error.
func (s *Server) evalIfMatch(w http.ResponseWriter, r *http.Request, object *object.Object) (bool, error) {
	ifMatch := r.Header.Get("If-Match")
	if ifMatch != "" && ifMatch != "*" {
		if ifMatchUint, err := strconv.ParseUint(ifMatch, 10, 64); err == nil {
			if ifMatchUint == object.ETag {
				return true, nil
			}
		} else {
			writeS3Error(w, http.StatusBadRequest, "InvalidRequest", "Invalid If-Match header format", object.Location.Key)
			return false, errors.New("Invalid If-Match header format")
		}
	}
	return true, nil
}

// Checks wether the If-Modified-Since header matches the object's last modified time. If it does, returns false, otherwise true.
// Writes S3 error on error.
func (s *Server) evalModifiedSince(w http.ResponseWriter, r *http.Request, object *object.Object) (bool, error) {
	ifModifiedSince := r.Header.Get("If-Modified-Since")
	if ifModifiedSince != "" {
		if t, err := time.Parse(time.RFC1123, ifModifiedSince); err == nil {
			if object.LastModified.After(t) {
				return true, nil
			}
		} else {
			writeS3Error(w, http.StatusBadRequest, "InvalidRequest", "Invalid If-Modified-Since header format", object.Location.Key)
			return false, errors.New("Invalid If-Modified-Since header format")
		}
	}
	return true, nil
}

// Checks wether the If-None-Match header matches the object's ETag. If it does, returns false, otherwise true.
func (s *Server) evalNoneMatch(w http.ResponseWriter, r *http.Request, object *object.Object) bool {
	ifNoneMatch := r.Header.Get("If-None-Match")
	if ifNoneMatch != "" {
		if ifNoneMatch != strconv.FormatUint(object.ETag, 10) {
			return true
		}
	}
	return true
}

// Checks wether the If-Unmodified-Since header matches the object's last modified time. If it does, returns false, otherwise true.
// Writes S3 error on error.
func (s *Server) evalIfUnmodifiedSince(w http.ResponseWriter, r *http.Request, object *object.Object) (bool, error) {
	ifUnmodifiedSince := r.Header.Get("If-Unmodified-Since")
	if ifUnmodifiedSince != "" {
		if t, err := time.Parse(time.RFC1123, ifUnmodifiedSince); err == nil {
			if object.LastModified.Before(t) {
				return true, nil
			}
		} else {
			writeS3Error(w, http.StatusBadRequest, "InvalidRequest", "Invalid If-Unmodified-Since header format", object.Location.Key)
			return false, errors.New("Invalid If-Unmodified-Since header format")
		}
	}
	return true, nil
}

// Checks wether the If-Match header matches the object's ETag. If it does, returns false, otherwise true.
// Writes S3 error on error.
func (s *Server) evalIfMatchLastModifiedTime(w http.ResponseWriter, r *http.Request, object *object.Object) (bool, error) {
	ifMatchLastModifiedTime := r.Header.Get("x-amz-if-match-last-modified-time")
	if ifMatchLastModifiedTime != "" {
		if t, err := time.Parse(time.RFC1123, ifMatchLastModifiedTime); err == nil {
			if object.LastModified.Equal(t) {
				return true, nil
			}
		} else {
			writeS3Error(w, http.StatusBadRequest, "InvalidRequest", "Invalid x-amz-if-match-last-modified-time header format", object.Location.Key)
			return false, errors.New("Invalid x-amz-if-match-last-modified-time header format")
		}
	}
	return true, nil
}

// Checks wether the x-amz-if-match-size header matches the object's size. If it does, returns false, otherwise true.
// Writes S3 error on error.
func (s *Server) evalIfMatchSize(w http.ResponseWriter, r *http.Request, object *object.Object) (bool, error) {
	ifMatchSize := r.Header.Get("x-amz-if-match-size")
	if ifMatchSize != "" && ifMatchSize != "*" {
		if ifMatchSizeUint, err := strconv.ParseUint(ifMatchSize, 10, 64); err == nil {
			if ifMatchSizeUint == uint64(object.Size) {
				return true, nil
			}
		} else {
			writeS3Error(w, http.StatusBadRequest, "InvalidRequest", "Invalid x-amz-if-match-size header format", object.Location.Key)
			return false, errors.New("Invalid x-amz-if-match-size header format")
		}
	}
	return true, nil
}

// true if matches the in the request specified conditions, false if not. Writes S3 error on error.
// Checks the If-Match, If-Modified-Since, If-None-Match, and If-Unmodified-Since headers against the object's ETag and last modified time.
// If any of the condition are not met, returns false and writes the appropriate S3 precondition failed error. If all conditions are met, returns true.
func (s *Server) evalIfMatches(w http.ResponseWriter, r *http.Request, object *object.Object) bool {
	ifMatch := r.Header.Get("If-Match")
	ifModifiedSince := r.Header.Get("If-Modified-Since")
	ifNoneMatch := r.Header.Get("If-None-Match")
	ifUnmodifiedSince := r.Header.Get("If-Unmodified-Since")

	ifMatchResult, err := s.evalIfMatch(w, r, object)
	if err != nil {
		return false
	}

	ifModifiedSinceResult, err := s.evalModifiedSince(w, r, object)
	if err != nil {
		return false
	}

	ifNoneMatchResult := s.evalNoneMatch(w, r, object)

	ifUnmodifiedSinceResult, err := s.evalIfUnmodifiedSince(w, r, object)
	if err != nil {
		return false
	}

	if ifMatch != "" {
		if ifUnmodifiedSince != "" {
			if !(ifMatchResult && ifUnmodifiedSinceResult == false) {
				writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match and If-Unmodified-Since conditions failed", object.Location.Key)
				return false
			}
		}
	} else if ifUnmodifiedSince != "" {
		if ifUnmodifiedSinceResult == false {
			writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Unmodified-Since condition failed", object.Location.Key)
			return false
		}
	}

	if ifNoneMatch != "" {
		if ifModifiedSince != "" {
			if !(ifNoneMatchResult && ifModifiedSinceResult == false) {
				writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-None-Match and If-Modified-Since conditions failed", object.Location.Key)
				return false
			}
		}
	} else if ifModifiedSince != "" {
		if ifModifiedSinceResult == false {
			writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Modified-Since condition failed", object.Location.Key)
			return false
		}
	}
	return true
}

func (s *Server) HeadObject(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	//partNumber := r.URL.Query().Get("partNumber") // Not supported yet
	ResponseCacheControl := r.URL.Query().Get("response-cache-control")
	ResponseContentDisposition := r.URL.Query().Get("response-content-disposition")
	ResponseContentEncoding := r.URL.Query().Get("response-content-encoding")
	ResponseContentLanguage := r.URL.Query().Get("response-content-language")
	ResponseContentType := r.URL.Query().Get("response-content-type")
	//ResponseExpires := r.URL.Query().Get("response-expires") // Not supported yet
	//VersionId := r.URL.Query().Get("versionId") // Not supported
	// Headers
	Range := r.Header.Get("Range")

	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket subdomain is required", bucketName)
		return
	}
	object, err := metadata.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: bucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}

	if !s.evalIfMatches(w, r, &object) {
		return
	}

	w.Header().Set("accept-ranges", Range)
	w.Header().Set("Last-Modified", object.LastModified.UTC().Format(time.RFC1123))
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	w.Header().Set("x-amz-checksum-xxhash64", strconv.FormatUint(object.ETag, 10))
	w.Header().Set("x-amz-checksum-type", "FULL_OBJECT")
	w.Header().Set("ETag", strconv.FormatUint(object.ETag, 10))
	w.Header().Set("Cache-Control", ResponseCacheControl)
	w.Header().Set("Content-Disposition", ResponseContentDisposition)
	w.Header().Set("Content-Encoding", ResponseContentEncoding)
	w.Header().Set("Content-Language", ResponseContentLanguage)
	w.Header().Set("Content-Type", ResponseContentType)
	w.Header().Set("Content-Range", Range)
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	w.Header().Set("x-amz-server-side-encryption", config.Cfg.EncryptionMethod)
	w.Header().Set("x-amz-storage-class", config.Cfg.StorageClass)
}

func (s *Server) PutObject(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket subdomain is required", bucketName)
		return
	}
	log.Printf("trace s3_put_object begin bucket=%s key=%s host=%s", bucketName, r.URL.Path, r.Host)
	object, err := objectManager.PutObjectStream(object.ObjectLocation{
		Bucket: object.Bucket{Name: bucketName},
		Key:    r.URL.Path,
	}, r.Body)
	if err != nil {
		log.Printf("trace s3_put_object failed bucket=%s key=%s err=%v", bucketName, r.URL.Path, err)
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}
	log.Printf("trace s3_put_object done bucket=%s key=%s etag=%d size=%d", bucketName, r.URL.Path, object.ETag, object.Size)

	w.Header().Set("ETag", strconv.FormatUint(object.ETag, 10))
	w.Header().Set("x-amz-checksum-type", "FULL_OBJECT")
	w.Header().Set("x-amz-checksum-xxhash64", strconv.FormatUint(object.ETag, 10))
	w.Header().Set("x-amz-object-size", strconv.FormatInt(object.Size, 10))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) GetObject(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	// Query parameters
	//partNumber := r.URL.Query().Get("partNumber")
	ResponseCacheControl := r.URL.Query().Get("response-cache-control")
	ResponseContentDisposition := r.URL.Query().Get("response-content-disposition")
	ResponseContentEncoding := r.URL.Query().Get("response-content-encoding")
	ResponseContentLanguage := r.URL.Query().Get("response-content-language")
	ResponseContentType := r.URL.Query().Get("response-content-type")
	ResponseExpires := r.URL.Query().Get("response-expires")
	//VersionId := r.URL.Query().Get("versionId") // Not supported
	// Headers
	//Range := r.Header.Get("Range")

	if ResponseContentType == "" {
		ResponseContentType = "application/octet-stream"
	}

	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket subdomain is required", bucketName)
		return
	}
	object, err := metadata.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: bucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}

	if !s.evalIfMatches(w, r, &object) {
		return
	}

	data, err := object.GetData()
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}
	w.Header().Set("Content-Type", ResponseContentType)
	w.Header().Set("Content-Disposition", ResponseContentDisposition)
	w.Header().Set("Content-Encoding", ResponseContentEncoding)
	w.Header().Set("Content-Language", ResponseContentLanguage)
	w.Header().Set("Content-Type", ResponseContentType)
	w.Header().Set("Cache-Control", ResponseCacheControl)
	w.Header().Set("Expires", ResponseExpires)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) ListObjectsV2(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	continuationToken, _ := strconv.Atoi(r.URL.Query().Get("continuation-token"))
	prefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")
	encodingType := r.URL.Query().Get("encoding-type")
	fetchOwner := r.URL.Query().Get("fetch-owner")
	startAfter := r.URL.Query().Get("start-after")

	maxKeys, _ := strconv.Atoi(r.URL.Query().Get("max-keys"))
	if maxKeys <= 0 || maxKeys > 1000 {
		maxKeys = 1000
	}

	println("bucket name:", bucketName, "max-keys:", maxKeys)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket subdomain is required", bucketName)
		return
	}
	objects, isTruncated, err := metadata.ListObjects(object.Bucket{Name: bucketName}, prefix, delimiter, startAfter, continuationToken, maxKeys)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}

	switch encodingType {
	case "url":
		delimiter = urlEncode(delimiter)
		prefix = urlEncode(prefix)
		startAfter = urlEncode(startAfter)
		for i := range objects {
			objects[i].Location.Key = urlEncode(string(objects[i].Location.Key))
		}
	}
	var commonPrefixes []CommonPrefix
	if delimiter != "" {
		for _, obj := range objects {
			truncatedKey := obj.Location.Key[len(prefix):]
			if !strings.Contains(truncatedKey, delimiter) {
				continue
			}
			commonPrefixes = append(commonPrefixes, CommonPrefix{Prefix: prefix + truncatedKey[:strings.Index(truncatedKey, delimiter)+1]})
		}
	}

	contents := make([]Contents, 0, len(objects))
	for _, obj := range objects {
		contents = append(contents, Contents{
			ChecksumAlgorithm: "XXHASH64",
			ChecksumType:      "FULL_OBJECT",
			ETag:              obj.ETag,
			Key:               string(obj.Location.Key),
			LastModified:      obj.LastModified.UTC().Format(time.RFC3339),
			Owner: Owner{
				ID:          config.Cfg.S3AccountID,
				DisplayName: config.Cfg.ServiceName,
			},
			Size:         obj.Size,
			StorageClass: config.Cfg.StorageClass,
		})
		if fetchOwner != "true" {
			contents[len(contents)-1].Owner = Owner{}
		}
	}

	response := ListBucketResult{
		Xmlns:          "http://s3.amazonaws.com/doc/2006-03-01/",
		CommonPrefixes: commonPrefixes,
		Contents:       contents,
		Delimiter:      delimiter,
		EncodingType:   encodingType,
		IsTruncated:    isTruncated,
		KeyCount:       len(objects),
		MaxKeys:        maxKeys,
		Name:           bucketName,
		Prefix:         prefix,
		StartAfter:     startAfter,
	}
	if continuationToken > 0 {
		response.ContinuationToken = strconv.Itoa(continuationToken)
	}
	if isTruncated {
		response.NextContinuationToken = strconv.Itoa(continuationToken + 1)
	}

	payload, err := xml.MarshalIndent(response, "", "  ")
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", bucketName)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}

func (s *Server) DeleteObject(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket subdomain is required", bucketName)
		return
	}

	obj, err := objectManager.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: bucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}
	match, err := s.evalIfMatch(w, r, &obj)
	if err != nil {
		return
	}
	if !match {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match condition failed", bucketName)
		return
	}

	match, err = s.evalIfMatchLastModifiedTime(w, r, &obj)
	if err != nil {
		return
	}
	if !match {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match-Last-Modified-Time condition failed", bucketName)
		return
	}

	match, err = s.evalIfMatchSize(w, r, &obj)
	if err != nil {
		return
	}
	if !match {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match-Size condition failed", bucketName)
		return
	}

	err = objectManager.DeleteObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: bucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}

	w.Header().Set("x-amz-delete-marker", "true")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteObjects(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket subdomain is required", bucketName)
		return
	}
	objectsToDelete := Delete{}
	err := xml.NewDecoder(r.Body).Decode(&objectsToDelete)
	if err != nil {
		writeS3Error(w, http.StatusBadRequest, "MalformedXML", "failed to parse request body", bucketName)
		return
	}

	var deletedObjects []DeletedObject
	var deleteErrors []DeleteError

	for _, obj := range objectsToDelete.Objects {
		err := objectManager.DeleteObject(object.ObjectLocation{
			Bucket: object.Bucket{Name: bucketName},
			Key:    obj.Key,
		})
		if err != nil {
			deleteErrors = append(deleteErrors, DeleteError{
				Code:    "InternalError",
				Key:     obj.Key,
				Message: err.Error(),
			})
		} else {
			deletedObjects = append(deletedObjects, DeletedObject{
				Key: obj.Key,
			})
		}
	}

	response := DeleteResult{
		Deleted: deletedObjects,
		Errors:  deleteErrors,
	}
	if objectsToDelete.Quiet {
		response.Deleted = nil
	}

	payload, err := xml.MarshalIndent(response, "", "  ")
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", bucketName)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}

func (s *Server) GetObjectAcl(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket subdomain is required", bucketName)
		return
	}
	_, err := objectManager.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: bucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), bucketName)
		return
	}

	acl := AccessControlPolicy{
		Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/",
		Owner: Owner{
			ID:          config.Cfg.S3AccountID,
			DisplayName: config.Cfg.ServiceName,
		},
		AccessControlList: []AccessControlList{
			{
				Grant: []Grantee{
					{
						Type:        "CanonicalUser",
						ID:          config.Cfg.S3AccountID,
						DisplayName: config.Cfg.ServiceName,
					},
				},
				Permission: "FULL_CONTROL",
			},
		},
	}

	payload, err := xml.MarshalIndent(acl, "", "  ")
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", bucketName)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}
