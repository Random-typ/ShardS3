package web

import (
	"encoding/xml"
	"io"
	"net/http"
	"shards3/services/shards3/internal/modules/s3/checksum"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/modules/storage/objectManager"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type InitiateMultipartUploadResult struct {
	Bucket   string `xml:"Bucket,omitempty"`
	Key      string `xml:"Key,omitempty"`
	UploadID string `xml:"UploadId,omitempty"`
}

type CompleteMultipartUploadPart struct {
	PartNumber int    `xml:"PartNumber,omitempty"`
	ETag       string `xml:"ETag,omitempty"`
}

type CompleteMultipartUpload struct {
	Parts []CompleteMultipartUploadPart `xml:"Part,omitempty"`
}

type CompleteMultipartUploadResult struct {
	Bucket string `xml:"Bucket,omitempty"`
	Key    string `xml:"Key,omitempty"`
	ETag   string `xml:"ETag,omitempty"`
}

type ListPartsResultPart struct {
	ETag         string `xml:"ETag,omitempty"`
	LastModified string `xml:"LastModified,omitempty"`
	PartNumber   int    `xml:"PartNumber,omitempty"`
	Size         int64  `xml:"Size,omitempty"`
}

type ListPartsResult struct {
	Bucket               string                `xml:"Bucket,omitempty"`
	Key                  string                `xml:"Key,omitempty"`
	UploadID             string                `xml:"UploadId,omitempty"`
	PartNumberMarker     int                   `xml:"PartNumberMarker,omitempty"`
	NextPartNumberMarker int                   `xml:"NextPartNumberMarker,omitempty"`
	MaxParts             int                   `xml:"MaxParts,omitempty"`
	IsTruncated          bool                  `xml:"IsTruncated,omitempty"`
	Parts                []ListPartsResultPart `xml:"Part,omitempty"`
	Initiator            User                  `xml:"Initiator,omitempty"`
	Owner                User                  `xml:"Owner,omitempty"`
	StorageClass         string                `xml:"StorageClass,omitempty"`
	ChecksumAlgorithm    string                `xml:"ChecksumAlgorithm,omitempty"`
	ChecksumType         string                `xml:"ChecksumType,omitempty"`
}

type ListMultipartUploadsResultUpload struct {
	ChecksumAlgorithm string `xml:"ChecksumAlgorithm,omitempty"`
	ChecksumType      string `xml:"ChecksumType,omitempty"`
	Initiated         string `xml:"Initiated,omitempty"`
	Initiator         User   `xml:"Initiator,omitempty"`
	Key               string `xml:"Key,omitempty"`
	Owner             User   `xml:"Owner,omitempty"`
	StorageClass      string `xml:"StorageClass,omitempty"`
	UploadID          string `xml:"UploadId,omitempty"`
}

type ListMultipartUploadsResult struct {
	Bucket             string                             `xml:"Bucket,omitempty"`
	KeyMarker          string                             `xml:"KeyMarker,omitempty"`
	UploadIDMarker     string                             `xml:"UploadIdMarker,omitempty"`
	NextKeyMarker      string                             `xml:"NextKeyMarker,omitempty"`
	Prefix             string                             `xml:"Prefix,omitempty"`
	Delimiter          string                             `xml:"Delimiter,omitempty"`
	NextUploadIDMarker string                             `xml:"NextUploadIdMarker,omitempty"`
	MaxUploads         int                                `xml:"MaxUploads,omitempty"`
	IsTruncated        bool                               `xml:"IsTruncated,omitempty"`
	Uploads            []ListMultipartUploadsResultUpload `xml:"Upload,omitempty"`
	Prefixes           []string                           `xml:"CommonPrefixes>Prefix,omitempty"`
	EncodingType       string                             `xml:"EncodingType,omitempty"`
}

type MultipartRequest struct {
	// special
	BucketName string `http:"Bucket,host" name:"bucket name"`
	Key        string `http:"Key,path" name:"object key"`

	// query
	UploadID         string `http:"uploadId,query" name:"upload ID"`
	PartNumber       int    `http:"partNumber,query" range:"1,10000" name:"part number"`
	Delimiter        string `http:"delimiter,query"`
	EncodingType     string `http:"encoding-type,query"`
	KeyMarker        string `http:"key-marker,query"`
	MaxUploads       int    `http:"max-uploads,query" range:"1,1000" default:"1000"`
	Prefix           string `http:"prefix,query"`
	UploadIDMarker   string `http:"upload-id-marker,query"`
	MaxParts         int    `http:"max-parts,query" range:"1,1000" default:"1000"`
	PartNumberMarker int    `http:"part-number-marker,query" range:"1,10000"`

	// headers
	IfMatch                 string    `http:"If-Match,header"`
	IfNoneMatch             string    `http:"If-None-Match,header"`
	IfMatchInitiatedTime    time.Time `http:"x-amz-if-match-initiated-time,header"`
	Acl                     string    `http:"x-amz-acl,header"`
	CacheControl            string    `http:"Cache-Control,header"`
	ContentDisposition      string    `http:"Content-Disposition,header"`
	ContentEncoding         string    `http:"Content-Encoding,header"`
	ContentLanguage         string    `http:"Content-Language,header"`
	ContentType             string    `http:"Content-Type,header"`
	ContentLength           int       `http:"Content-Length,header"`
	Expires                 time.Time `http:"Expires,header"`
	GrantFullControl        string    `http:"x-amz-grant-full-control,header"`
	GrantRead               string    `http:"x-amz-grant-read,header"`
	GrantReadACP            string    `http:"x-amz-grant-read-acp,header"`
	GrantWriteACP           string    `http:"x-amz-grant-write-acp,header"`
	ServerSideEncryption    string    `http:"x-amz-server-side-encryption,header"`
	StorageClass            string    `http:"x-amz-storage-class,header"`
	WebsiteRedirectLocation string    `http:"x-amz-website-redirect-location,header"`
	Tagging                 string    `http:"x-amz-tagging,header"`
	ChecksumAlgorithm       string    `http:"x-amz-checksum-algorithm,header"`
	ChecksumType            string    `http:"x-amz-checksum-type,header"`
	MultipartObjectSize     int       `http:"x-amz-mp-object-size,header"`
}

type MultipartResponse struct {
	ChecksumAlgorithm string `http:"x-amz-checksum-algorithm"`
	ChecksumType      string `http:"x-amz-checksum-type"`
	ETag              uint64 `http:"ETag"`
}

// Returns the object location.
// Does not validate the bucket name or key, just returns them as-is.
func (s *MultipartRequest) GetObjectLocation() object.ObjectLocation {
	return object.ObjectLocation{
		Bucket: object.Bucket{Name: s.BucketName},
		Key:    s.Key,
	}
}

// Checks wether the bucket and key in the request match the bucket and key in the multipart upload.
// Handles S3 error response
// Returns true if they match, false otherwise.
func (s *MultipartRequest) BucketAndKeyMatch(w http.ResponseWriter, r *http.Request, multipartUpload *object.MultipartUpload) bool {
	if multipartUpload.Location.Bucket.Name != s.BucketName || multipartUpload.Location.Key != s.Key {
		writeS3Error(w, http.StatusBadRequest, "InvalidUploadId", "upload ID does not match bucket and key", s.Key)
		return false
	}
	return true
}

// Checks wether the ETag in the request matches the ETag of the object.
// Handles S3 precondition error response
// Returns true if they match, false otherwise.
func (s *MultipartRequest) ETagMatch(w http.ResponseWriter, r *http.Request, object *object.Object) bool {
	if s.IfMatch != "" && s.IfMatch != strconv.FormatUint(object.ETag, 10) {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "ETag does not match", s.Key)
		return false
	}
	return true
}

// Checks wether both specified times match. If either time is nil, it is considered a match.
// Handles S3 precondition error response
// Returns true if they match, false otherwise.
func (s *MultipartRequest) TimeMatch(w http.ResponseWriter, r *http.Request, x *time.Time, y *time.Time, err string) bool {
	if x == nil || y == nil {
		return true
	}
	if !x.Equal(*y) {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", err, s.Key)
		return false
	}
	return true
}

// Checks wether the key in the request matches the specified key.
// Handles S3 precondition error response
// Returns true if they match, false otherwise.
func (s *MultipartRequest) KeyMatch(w http.ResponseWriter, r *http.Request, key string, err string) bool {
	if s.Key != key {
		writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", err, s.Key)
		return false
	}
	return true
}

func (s *Server) CreateMultipartUpload(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[MultipartRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}

	exists, err := metadata.ObjectExists(req.GetObjectLocation())
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}
	if exists {
		writeS3Error(w, http.StatusConflict, "ObjectAlreadyExists", "object already exists", req.Key)
		return
	}

	multipartUpload, err := objectManager.CreateMultipartUpload(req.GetObjectLocation())
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	response := InitiateMultipartUploadResult{
		Bucket:   req.BucketName,
		Key:      req.Key,
		UploadID: multipartUpload.UploadID,
	}

	headers := MultipartResponse{
		ChecksumAlgorithm: GetDefaultChecksumMetadata().Algorithm,
		ChecksumType:      GetDefaultChecksumMetadata().Type,
	}

	WriteResponse(w, http.StatusOK, headers, response, nil)
}

func (s *Server) UploadPart(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[MultipartRequest](w, r, true, true, nil, []string{"partNumber", "uploadId"})
	if req == nil {
		return
	}

	multipartUpload, err := metadata.GetMultipartUpload(req.UploadID)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	if !req.BucketAndKeyMatch(w, r, &multipartUpload) {
		return
	}

	checksumR, checksumW := io.Pipe()
	processR, processW := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := checksum.VerifyChecksums(w, r, checksumR)
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
			return
		}
	}()

	var obj object.MultipartPart
	go func() {
		defer wg.Done()
		var err error
		obj, err = objectManager.UploadPartStream(multipartUpload.Location, req.PartNumber, req.UploadID, processR)
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
			return
		}
	}()

	buf := make([]byte, 64*1024)
	_, err = io.CopyBuffer(io.MultiWriter(checksumW, processW), r.Body, buf)

	checksumW.CloseWithError(err)
	processW.CloseWithError(err)

	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	wg.Wait()

	if err := checksum.AddChecksumHeaders(w, r); err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}
	headers := MultipartResponse{
		ETag: obj.ETag,
	}
	WriteResponse(w, http.StatusOK, headers, nil, nil)
}

func (s *Server) CompleteMultipartUpload(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[MultipartRequest](w, r, true, true, nil, []string{"uploadId"})
	if req == nil {
		return
	}

	multipartUpload, err := metadata.GetMultipartUpload(req.UploadID)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	if req.IfNoneMatch == "*" && req.KeyMatch(w, r, multipartUpload.Location.Key, "If-None-Match header is set to '*', but the object already exists") {
		return
	}

	if !req.BucketAndKeyMatch(w, r, &multipartUpload) {
		return
	}

	obj, err := metadata.CompleteMultipartUpload(req.UploadID)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	if req.IfMatch != "" && !req.ETagMatch(w, r, &obj) {
		return
	}

	// parse the XML body to get the list of parts
	var completeMultipartUpload CompleteMultipartUpload
	err = xml.NewDecoder(r.Body).Decode(&completeMultipartUpload)
	if err != nil {
		writeS3Error(w, http.StatusBadRequest, "MalformedXML", "The XML you provided was not well-formed or did not validate against our published schema", req.Key)
		return
	}
	parts, _, err := metadata.ListParts(req.UploadID, 10000, 1)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}
	// sort by part number for faster check
	sort.Slice(completeMultipartUpload.Parts, func(i, j int) bool {
		return completeMultipartUpload.Parts[i].PartNumber < completeMultipartUpload.Parts[j].PartNumber
	})
	// verify that all parts in the completeMultipartUpload are present and their checksums match.
	for _, part := range parts {
		var index = part.PartNumber - 1
		if len(completeMultipartUpload.Parts) > index {
			// perform binary search on completeMultipartUpload.Parts to find part number
			index = sort.Search(len(completeMultipartUpload.Parts), func(i int) bool {
				return completeMultipartUpload.Parts[i].PartNumber >= part.PartNumber
			})
			if index >= len(completeMultipartUpload.Parts) || completeMultipartUpload.Parts[index].PartNumber != part.PartNumber {
				writeS3Error(w, http.StatusBadRequest, "InvalidPart", "part number "+strconv.Itoa(part.PartNumber)+" is missing in the complete multipart upload request", req.Key)
				return
			}
		}
		etagInt, err := strconv.ParseUint(completeMultipartUpload.Parts[index].ETag, 10, 64)
		if err != nil {
			writeS3Error(w, http.StatusBadRequest, "InvalidETag", "ETag is not a valid integer", req.Key)
			return
		}
		if etagInt != part.ETag {
			writeS3Error(w, http.StatusBadRequest, "InvalidETag", "ETag does not match for part number "+strconv.Itoa(part.PartNumber), req.Key)
			return
		}
	}

	response := CompleteMultipartUploadResult{
		Bucket: req.BucketName,
		Key:    req.Key,
		ETag:   strconv.FormatUint(obj.ETag, 10),
	}

	WriteResponse(w, http.StatusOK, nil, response, nil)
}

func (s *Server) AbortMultipartUpload(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[MultipartRequest](w, r, true, true, nil, []string{"uploadId"})
	if req == nil {
		return
	}

	multipartUpload, err := metadata.GetMultipartUpload(req.UploadID)
	if err != nil {
		if !req.IfMatchInitiatedTime.IsZero() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	if !req.BucketAndKeyMatch(w, r, &multipartUpload) {
		return
	}
	if !req.IfMatchInitiatedTime.IsZero() && !req.TimeMatch(w, r, &req.IfMatchInitiatedTime, &multipartUpload.Initiated, "If-Match-Initiated-Time header does not match the upload initiated time") {
		return
	}

	err = objectManager.AbortMultipartUpload(req.UploadID)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	WriteResponse(w, http.StatusNoContent, nil, nil, nil)
}

func (s *Server) ListMultipartUploads(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[MultipartRequest](w, r, true, true, nil, nil)
	if req == nil {
		return
	}
	uploads, hasMore, err := metadata.ListMultipartUploads(object.Bucket{Name: req.BucketName}, req.Prefix, req.Delimiter, req.KeyMarker, req.UploadIDMarker, req.MaxUploads)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	commonPrefixes := make(map[string]struct{})

	if req.Delimiter != "" && req.Prefix != "" {
		for i := 0; i < len(uploads); i++ {
			upload := uploads[i]
			end := strings.Index(upload.Location.Key[len(req.Prefix):], req.Delimiter)
			if end != -1 {
				// Remove the upload from the list of uploads, since it is a common prefix.
				uploads = append(uploads[:i], uploads[i+1:]...)
				i--
				commonPrefixes[upload.Location.Key[:len(req.Prefix)+end]] = struct{}{}
			}
		}
	}

	uploadsResult := make([]ListMultipartUploadsResultUpload, len(uploads))
	for i, upload := range uploads {
		uploadsResult[i] = ListMultipartUploadsResultUpload{
			ChecksumAlgorithm: GetDefaultChecksumMetadata().Algorithm,
			ChecksumType:      GetDefaultChecksumMetadata().Type,
			Initiator:         GetDefaultUser(),
			Owner:             GetDefaultUser(),
			StorageClass:      GetDefaultStorageClass(),
			UploadID:          upload.UploadID,
			Key:               upload.Location.Key,
			Initiated:         upload.Initiated.UTC().Format(time.RFC3339),
		}
	}

	response := ListMultipartUploadsResult{
		KeyMarker:      req.KeyMarker,
		UploadIDMarker: req.UploadIDMarker,
		Prefix:         req.Prefix,
		Delimiter:      req.Delimiter,
		MaxUploads:     req.MaxUploads,
		IsTruncated:    hasMore,
		Uploads:        uploadsResult,
		Bucket:         req.BucketName,
	}
	if len(commonPrefixes) > 0 {
		response.Prefixes = make([]string, 0, len(commonPrefixes))
		for prefix := range commonPrefixes {
			response.Prefixes = append(response.Prefixes, prefix)
		}
	}

	if req.EncodingType == "url" {
		response.EncodingType = req.EncodingType
		response.KeyMarker = urlEncode(response.KeyMarker)
		response.NextKeyMarker = urlEncode(response.NextKeyMarker)
		response.Delimiter = urlEncode(response.Delimiter)
		response.Prefix = urlEncode(response.Prefix)
		for i, upload := range response.Uploads {
			response.Uploads[i].Key = urlEncode(upload.Key)
		}
	}

	if hasMore && len(uploads) > 0 {
		response.NextKeyMarker = uploads[len(uploads)-1].Location.Key
		response.NextUploadIDMarker = uploads[len(uploads)-1].UploadID
	}
	WriteResponse(w, http.StatusOK, nil, response, nil)
}

func (s *Server) ListParts(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[MultipartRequest](w, r, true, true, nil, []string{"uploadId"})
	if req == nil {
		return
	}

	multipartUpload, err := metadata.GetMultipartUpload(req.UploadID)
	if err != nil {
		if !req.IfMatchInitiatedTime.IsZero() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	if !req.BucketAndKeyMatch(w, r, &multipartUpload) {
		return
	}

	metadataParts, hasMore, err := metadata.ListParts(req.UploadID, req.MaxParts, req.PartNumberMarker)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), req.Key)
		return
	}

	Parts := make([]ListPartsResultPart, len(metadataParts))
	for i, part := range metadataParts {
		Parts[i] = ListPartsResultPart{
			PartNumber:   part.PartNumber,
			ETag:         strconv.FormatUint(part.ETag, 10),
			Size:         part.Size,
			LastModified: part.CreatedAt.UTC().Format(time.RFC3339),
		}
	}

	response := ListPartsResult{
		Bucket:            multipartUpload.Location.Bucket.Name,
		Key:               multipartUpload.Location.Key,
		UploadID:          multipartUpload.UploadID,
		PartNumberMarker:  req.PartNumberMarker,
		MaxParts:          req.MaxParts,
		IsTruncated:       hasMore,
		Parts:             Parts,
		Initiator:         GetDefaultUser(),
		Owner:             GetDefaultUser(),
		StorageClass:      GetDefaultStorageClass(),
		ChecksumAlgorithm: GetDefaultChecksumMetadata().Algorithm,
		ChecksumType:      GetDefaultChecksumMetadata().Type,
	}

	if hasMore {
		response.NextPartNumberMarker = metadataParts[len(metadataParts)-1].PartNumber + 1
	}

	WriteResponse(w, http.StatusOK, nil, response, nil)
}
