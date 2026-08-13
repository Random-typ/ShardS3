package web

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/modules/storage/objectManager"
	"strconv"
	"strings"
	"time"
)

type ListBucketResult struct {
	XMLName     xml.Name `xml:"ListBucketResult"`
	IsTruncated bool     `xml:"IsTruncated,omitempty"`

	Contents []Contents `xml:"Contents,omitempty"`

	Name                  string         `xml:"Name,omitempty"`
	Prefix                string         `xml:"Prefix,omitempty"`
	Delimiter             string         `xml:"Delimiter,omitempty"`
	MaxKeys               int            `xml:"MaxKeys,omitempty"`
	CommonPrefixes        []CommonPrefix `xml:"CommonPrefixes,omitempty"`
	EncodingType          string         `xml:"EncodingType,omitempty"`
	KeyCount              int            `xml:"KeyCount,omitempty"`
	ContinuationToken     int            `xml:"ContinuationToken,omitempty"`
	NextContinuationToken int            `xml:"NextContinuationToken,omitempty"`
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
	Owner               User   `xml:"Owner,omitempty"`
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

type ObjectRequest struct {
	// special
	BucketName string `http:"Bucket,host" name:"bucket name"`
	Key        string `http:"Key,path" name:"object key"`

	CacheControl       string    `http:"response-cache-control,query"`
	ContentDisposition string    `http:"response-content-disposition,query"`
	ContentEncoding    string    `http:"response-content-encoding,query"`
	ContentLanguage    string    `http:"response-content-language,query"`
	ContentType        string    `http:"response-content-type,query"`
	PartNumber         int       `http:"part-number,query" range:"1,10000"`
	Expires            time.Time `http:"response-expires,query"`
	Prefix             string    `http:"prefix,query"`
	ContinuationToken  int       `http:"continuation-token,query"`
	Delimiter          string    `http:"delimiter,query"`
	FetchOwner         string    `http:"fetch-owner,query"`
	StartAfter         string    `http:"start-after,query"`
	EncodingType       string    `http:"encoding-type,query"`
	MaxKeys            int       `http:"max-keys,query" range:"1,1000"`

	IfMatch           string `http:"If-Match,header"`
	IfModifiedSince   string `http:"If-Modified-Since,header"`
	IfNoneMatch       string `http:"If-None-Match,header"`
	IfUnmodifiedSince string `http:"If-Unmodified-Since,header"`
	IfMatchSize       string `http:"x-amz-if-match-size,header"`
	Range             string `http:"range,header"`
}

type ObjectResponse struct {
	AcceptRanges       string    `http:"accept-ranges"`
	LastModified       time.Time `http:"last-modified"`
	ContentLength      int64     `http:"Content-Length"`
	xxHash64           uint64    `http:"x-amz-checksum-xxhash64"`
	ChecksumType       string    `http:"x-amz-checksum-type"`
	ETag               uint64    `http:"ETag"`
	CacheControl       string    `http:"Cache-Control"`
	ContentDisposition string    `http:"Content-Disposition"`
	ContentEncoding    string    `http:"Content-Encoding"`
	ContentLanguage    string    `http:"Content-Language"`
	ContentType        string    `http:"Content-Type"`
	DeleteMarker       string    `http:"x-amz-delete-marker"`
	ContentRange       string    `http:"Content-Range"`
	StorageClass       string    `http:"x-amz-storage-class"`
	ObjectSize         int64     `http:"x-amz-object-size"`
	Expires            time.Time `http:"Expires"`
}

// Checks wether the If-Match header matches the object's ETag. If it does, returns false, otherwise true.
// Writes S3 error on error.
func (s *Server) evalIfMatch(w http.ResponseWriter, r *http.Request, object *object.Object, req *ObjectRequest) (bool, error) {
	if req.IfMatch != "" && req.IfMatch != "*" {
		if ifMatchUint, err := strconv.ParseUint(req.IfMatch, 10, 64); err == nil {
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
func (s *Server) evalModifiedSince(w http.ResponseWriter, r *http.Request, object *object.Object, req *ObjectRequest) (bool, error) {
	if req.IfModifiedSince != "" {
		if t, err := time.Parse(time.RFC1123, req.IfModifiedSince); err == nil {
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
func (s *Server) evalNoneMatch(w http.ResponseWriter, r *http.Request, object *object.Object, req *ObjectRequest) bool {
	if req.IfNoneMatch != "" {
		if req.IfNoneMatch != strconv.FormatUint(object.ETag, 10) {
			return true
		}
	}
	return true
}

// Checks wether the If-Unmodified-Since header matches the object's last modified time. If it does, returns false, otherwise true.
// Writes S3 error on error.
func (s *Server) evalIfUnmodifiedSince(w http.ResponseWriter, r *http.Request, object *object.Object, req *ObjectRequest) (bool, error) {
	if req.IfUnmodifiedSince != "" {
		if t, err := time.Parse(time.RFC1123, req.IfUnmodifiedSince); err == nil {
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
func (s *Server) evalIfMatchLastModifiedTime(w http.ResponseWriter, r *http.Request, object *object.Object, req *ObjectRequest) (bool, error) {
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
func (s *Server) evalIfMatchSize(w http.ResponseWriter, r *http.Request, object *object.Object, req *ObjectRequest) (bool, error) {
	if req.IfMatchSize != "" && req.IfMatchSize != "*" {
		if ifMatchSizeUint, err := strconv.ParseUint(req.IfMatchSize, 10, 64); err == nil {
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
func (s *Server) evalIfMatches(w http.ResponseWriter, r *http.Request, object *object.Object, req *ObjectRequest) bool {
	ifMatchResult, err := s.evalIfMatch(w, r, object, req)
	if err != nil {
		return false
	}

	ifModifiedSinceResult, err := s.evalModifiedSince(w, r, object, req)
	if err != nil {
		return false
	}

	ifNoneMatchResult := s.evalNoneMatch(w, r, object, req)

	ifUnmodifiedSinceResult, err := s.evalIfUnmodifiedSince(w, r, object, req)
	if err != nil {
		return false
	}

	if req.IfMatch != "" {
		if req.IfUnmodifiedSince != "" {
			if !(ifMatchResult && !ifUnmodifiedSinceResult) {
				writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match and If-Unmodified-Since conditions failed", object.Location.Key)
				return false
			}
		}
	} else if req.IfUnmodifiedSince != "" {
		if !ifUnmodifiedSinceResult {
			writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Unmodified-Since condition failed", object.Location.Key)
			return false
		}
	}

	if req.IfNoneMatch != "" {
		if req.IfModifiedSince != "" {
			if !(ifNoneMatchResult && !ifModifiedSinceResult) {
				writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-None-Match and If-Modified-Since conditions failed", object.Location.Key)
				return false
			}
		}
	} else if req.IfModifiedSince != "" {
		if !ifModifiedSinceResult {
			writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Modified-Since condition failed", object.Location.Key)
			return false
		}
	}
	return true
}

func (s *Server) HeadObject(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	headers := ObjectResponse{
		ChecksumType:       GetDefaultChecksumMetadata().Type,
		CacheControl:       req.CacheControl,
		ContentDisposition: req.ContentDisposition,
		ContentEncoding:    req.ContentEncoding,
		ContentLanguage:    req.ContentLanguage,
		ContentType:        req.ContentType,
		StorageClass:       GetDefaultStorageClass(),
	}

	if req.PartNumber != 0 {
		_, err := metadata.GetMultipartUploadByLocation(object.ObjectLocation{
			Bucket: object.Bucket{Name: req.BucketName},
			Key:    r.URL.Path,
		})
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
			return
		}

	}

	object, err := metadata.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: req.BucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}

	if !s.evalIfMatches(w, r, &object, req) {
		return
	}

	var start int64 = 0
	var end int64 = object.Size
	if req.Range != "" {
		start, end, _, err := ParseContentRangeHeader(req.Range)
		if err != nil {
			writeS3Error(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", err.Error(), object.Location.Key)
			return
		}
		if start < 0 || start >= end || end > object.Size {
			writeS3Error(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Requested Range Not Satisfiable", object.Location.Key)
			return
		}
	}

	headers.ContentLength = end - start
	headers.LastModified = object.LastModified
	headers.xxHash64 = object.ETag
	headers.ETag = object.ETag

	if req.Range != "" {
		headers.AcceptRanges = "bytes"
	}

	WriteResponse(w, http.StatusOK, headers, nil, nil)
}

func (s *Server) PutObject(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	object, err := objectManager.PutObjectStream(object.ObjectLocation{
		Bucket: object.Bucket{Name: req.BucketName},
		Key:    r.URL.Path,
	}, r.Body)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}

	headers := ObjectResponse{
		xxHash64:     object.ETag,
		ChecksumType: GetDefaultChecksumMetadata().Type,
		ETag:         object.ETag,
		ObjectSize:   object.Size,
	}

	WriteResponse(w, http.StatusOK, headers, nil, nil)
}

func (s *Server) GetObject(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	obj, err := metadata.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: req.BucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}

	if !s.evalIfMatches(w, r, &obj, req) {
		return
	}

	var start int64 = 0
	var end int64 = 0
	if req.Range != "" {
		start, end, _, err := ParseContentRangeHeader(req.Range)
		if err != nil {
			writeS3Error(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", err.Error(), req.BucketName)
			return
		}
		if start < 0 || start >= end || end > obj.Size {
			writeS3Error(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "Invalid range specified", req.BucketName)
			return
		}
	}

	headers := ObjectResponse{
		ContentType:        req.ContentType,
		ContentDisposition: req.ContentDisposition,
		ContentEncoding:    req.ContentEncoding,
		ContentLanguage:    req.ContentLanguage,
		CacheControl:       req.CacheControl,
		Expires:            req.Expires,
		ContentLength:      end - start,
	}

	var data []byte
	if req.PartNumber != 0 {
		multipartUpload, err := metadata.GetMultipartUploadByLocation(object.ObjectLocation{
			Bucket: object.Bucket{Name: req.BucketName},
			Key:    r.URL.Path,
		})
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
			return
		}
		part, err := metadata.GetPart(multipartUpload.UploadID, req.PartNumber)
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
			return
		}
		data, err = multipartUpload.GetData(part, start, end)
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
			return
		}
	} else {
		data, err = obj.GetData(start, end)
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
			return
		}
	}

	if req.Range != "" {
		headers.ContentRange = fmt.Sprintf("bytes %d-%d/%d", start, end-1, obj.Size)
	}

	WriteResponse(w, http.StatusOK, headers, nil, data)
}

func (s *Server) ListObjectsV2(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	objects, isTruncated, err := metadata.ListObjects(object.Bucket{Name: req.BucketName}, req.Prefix, req.Delimiter, req.StartAfter, req.ContinuationToken, req.MaxKeys)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}

	if req.EncodingType == "url" {
		req.Delimiter = urlEncode(req.Delimiter)
		req.Prefix = urlEncode(req.Prefix)
		req.StartAfter = urlEncode(req.StartAfter)
		for i := range objects {
			objects[i].Location.Key = urlEncode(string(objects[i].Location.Key))
		}
	}
	var commonPrefixes []CommonPrefix
	if req.Delimiter != "" {
		for _, obj := range objects {
			truncatedKey := obj.Location.Key[len(req.Prefix):]
			if !strings.Contains(truncatedKey, req.Delimiter) {
				continue
			}
			commonPrefixes = append(commonPrefixes, CommonPrefix{Prefix: req.Prefix + truncatedKey[:strings.Index(truncatedKey, req.Delimiter)+1]})
		}
	}

	contents := make([]Contents, 0, len(objects))
	for _, obj := range objects {
		contents = append(contents, Contents{
			ChecksumAlgorithm: GetDefaultChecksumMetadata().Algorithm,
			ChecksumType:      GetDefaultChecksumMetadata().Type,
			ETag:              obj.ETag,
			Key:               string(obj.Location.Key),
			LastModified:      obj.LastModified.UTC().Format(time.RFC3339),
			Owner:             GetDefaultUser(),
			Size:              obj.Size,
			StorageClass:      GetDefaultStorageClass(),
		})
		if req.FetchOwner != "true" {
			contents[len(contents)-1].Owner = User{}
		}
	}

	response := ListBucketResult{
		CommonPrefixes:    commonPrefixes,
		Contents:          contents,
		Delimiter:         req.Delimiter,
		EncodingType:      req.EncodingType,
		IsTruncated:       isTruncated,
		KeyCount:          len(objects),
		MaxKeys:           req.MaxKeys,
		Name:              req.BucketName,
		Prefix:            req.Prefix,
		StartAfter:        req.StartAfter,
		ContinuationToken: req.ContinuationToken,
	}
	if isTruncated {
		response.NextContinuationToken = req.ContinuationToken + 1
	}

	WriteResponse(w, http.StatusOK, nil, response, nil)
}

func (s *Server) DeleteObject(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	obj, err := objectManager.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: req.BucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}
	match, err := s.evalIfMatch(w, r, &obj, req)
	if err != nil {
		return
	}
	if !match {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match condition failed", req.BucketName)
		return
	}

	match, err = s.evalIfMatchLastModifiedTime(w, r, &obj, req)
	if err != nil {
		return
	}
	if !match {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match-Last-Modified-Time condition failed", req.BucketName)
		return
	}

	match, err = s.evalIfMatchSize(w, r, &obj, req)
	if err != nil {
		return
	}
	if !match {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "If-Match-Size condition failed", req.BucketName)
		return
	}

	err = objectManager.DeleteObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: req.BucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}

	w.Header().Set("x-amz-delete-marker", "true")

	headers := ObjectResponse{
		DeleteMarker: "true",
	}

	WriteResponse(w, http.StatusOK, headers, nil, nil)
}

func (s *Server) DeleteObjects(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	objectsToDelete := Delete{}
	err := xml.NewDecoder(r.Body).Decode(&objectsToDelete)
	if err != nil {
		writeS3Error(w, http.StatusBadRequest, "MalformedXML", "failed to parse request body", req.BucketName)
		return
	}

	var deletedObjects []DeletedObject
	var deleteErrors []DeleteError

	for _, obj := range objectsToDelete.Objects {
		err := objectManager.DeleteObject(object.ObjectLocation{
			Bucket: object.Bucket{Name: req.BucketName},
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

	WriteResponse(w, http.StatusOK, nil, response, nil)
}

func (s *Server) GetObjectAcl(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	_, err := objectManager.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: req.BucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}

	response := AccessControlPolicy{
		Owner: GetDefaultUser(),
		AccessControlList: []AccessControlList{
			{
				Grant: []Grantee{
					{
						Type:        "CanonicalUser",
						ID:          GetDefaultUser().ID,
						DisplayName: GetDefaultUser().DisplayName,
					},
				},
				Permission: "FULL_CONTROL",
			},
		},
	}

	WriteResponse(w, http.StatusOK, nil, response, nil)

}

func (s *Server) GetObjectTagging(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[ObjectRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	_, err := objectManager.GetObject(object.ObjectLocation{
		Bucket: object.Bucket{Name: req.BucketName},
		Key:    r.URL.Path,
	})
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.BucketName)
		return
	}
	response := Tagging{
		TagSet: nil,
	}

	WriteResponse(w, http.StatusOK, nil, response, nil)
}
