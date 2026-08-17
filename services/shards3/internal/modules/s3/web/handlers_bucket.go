package web

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"shards3/services/shards3/internal/modules/s3/ARN"
	"shards3/services/shards3/internal/platform/config"
	"strconv"
	"time"
)

type ListAllMyBucketsResult struct {
	XMLName xml.Name `xml:"ListAllMyBucketsResult"`

	Buckets           []Bucket `xml:"Buckets>Bucket,omitempty"`
	Owner             User     `xml:"Owner,omitempty"`
	ContinuationToken string   `xml:"ContinuationToken,omitempty"`
	Prefix            string   `xml:"Prefix,omitempty"`
}

type User struct {
	ID          string `xml:"ID,omitempty"`
	DisplayName string `xml:"DisplayName,omitempty"`
}

type Bucket struct {
	XMLName xml.Name `xml:"Bucket"`

	BucketArn    string `xml:"BucketArn,omitempty"`
	BucketRegion string `xml:"BucketRegion,omitempty"`
	CreationDate string `xml:"CreationDate,omitempty"`
	Name         string `xml:"Name,omitempty"`
}

type LocationConstraint struct {
	XMLName xml.Name `xml:"LocationConstraint"`

	LocationConstraint string `xml:"LocationConstraint,omitempty"`
}

type Rule struct {
	XMLName xml.Name `xml:"Rule"`

	SSEAlgorithm     string   `xml:"ApplyServerSideEncryptionByDefault>SSEAlgorithm,omitempty"`
	KMSMasterKeyID   string   `xml:"ApplyServerSideEncryptionByDefault>KMSMasterKeyID,omitempty"`
	EncryptionType   []string `xml:"BlockedEncryptionTypes>EncryptionType,omitempty"`
	BucketKeyEnabled bool     `xml:"BucketKeyEnabled,omitempty"`
}

type ServerSideEncryptionConfiguration struct {
	XMLName xml.Name `xml:"ServerSideEncryptionConfiguration"`
	Rules   []Rule   `xml:"Rule,omitempty"`
}

type Grantee struct {
	XMLName xml.Name `xml:"Grantee"`

	DisplayName  string `xml:"DisplayName,omitempty"`
	EmailAddress string `xml:"EmailAddress,omitempty"`
	ID           string `xml:"ID,omitempty"`
	Type         string `xml:"xsi:type,attr,omitempty"`
	URI          string `xml:"URI,omitempty"`
}

type AccessControlList struct {
	XMLName xml.Name `xml:"AccessControlList"`

	Grant      []Grantee `xml:"Grant>Grantee,omitempty"`
	Permission string    `xml:"Permission,omitempty"`
}

type AccessControlPolicy struct {
	XMLName xml.Name `xml:"AccessControlPolicy"`

	Owner             User                `xml:"Owner,omitempty"`
	AccessControlList []AccessControlList `xml:"AccessControlList>Grant,omitempty"`
}

type Tag struct {
	XMLName xml.Name `xml:"Tag"`

	Key   string `xml:"Key,omitempty"`
	Value string `xml:"Value,omitempty"`
}

type TagSet struct {
	XMLName xml.Name `xml:"TagSet"`

	Tags []Tag `xml:"Tag,omitempty"`
}

type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`

	TagSet TagSet `xml:"TagSet,omitempty"`
}

type BucketRequest struct {
	BucketName string `http:"Bucket,host" name:"bucket name"`

	Prefix            string `http:"prefix,query"`
	MaxBuckets        int    `http:"max-buckets,query" range:"1,10000"`
	ContinuationToken int    `http:"continuation-token,query" default:"1"`
}

type BucketResponse struct {
	BucketArn    string `http:"x-amz-bucket-arn" name:"bucket arn"`
	BucketRegion string `http:"x-amz-bucket-region" name:"bucket region"`
}

func urlEncode(s string) string {
	return url.QueryEscape(s)
}

func (s *Server) GetBucketLocation(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[BucketRequest](w, r, true, false, nil, nil)
	if req == nil {
		return
	}

	_, err := s.bucketService.GetBucket(req.BucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}

	response := LocationConstraint{
		LocationConstraint: config.Cfg.S3Region,
	}

	payload, err := xml.MarshalIndent(response, "", "  ")
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}

func (s *Server) HeadBucket(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[BucketRequest](w, r, true, false, nil, nil)
	if req == nil {
		return
	}

	_, err := s.bucketService.GetBucket(req.BucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}
	res := BucketResponse{
		BucketArn:    ARN.GetBucketArn(req.BucketName),
		BucketRegion: config.Cfg.S3Region,
	}

	WriteResponse(w, http.StatusOK, &res, nil, nil)
}

func (s *Server) ListBuckets(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[BucketRequest](w, r, true, false, nil, nil)
	if req == nil {
		return
	}

	buckets, hasMore, err := s.bucketService.ListBuckets(req.Prefix, req.ContinuationToken, req.MaxBuckets)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), r.URL.Path)
		return
	}
	if hasMore {
		req.ContinuationToken++
	} else {
		req.ContinuationToken = 0
	}

	items := make([]Bucket, 0, len(buckets))
	for _, bucket := range buckets {
		created := bucket.CreatedAt.UTC()
		if created.IsZero() {
			created = time.Now().UTC()
		}
		items = append(items, Bucket{Name: bucket.Name, CreationDate: created.Format(time.RFC3339)})
	}

	response := ListAllMyBucketsResult{
		Owner:   GetDefaultUser(),
		Buckets: items,
		Prefix:  req.Prefix,
	}

	if req.ContinuationToken != 0 {
		response.ContinuationToken = strconv.Itoa(req.ContinuationToken)
	}

	WriteResponse(w, http.StatusOK, nil, response, nil)
}

func (s *Server) GetBucketEncryption(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[BucketRequest](w, r, true, false, nil, nil)
	if req == nil {
		return
	}

	_, err := s.bucketService.GetBucket(req.BucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}
	response := ServerSideEncryptionConfiguration{
		Rules: []Rule{
			{
				SSEAlgorithm:     config.Cfg.EncryptionMethod,
				BucketKeyEnabled: false,
			},
		},
	}

	WriteResponse(w, http.StatusOK, nil, response, nil)
}

func (s *Server) GetBucketAcl(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[BucketRequest](w, r, true, false, nil, nil)
	if req == nil {
		return
	}

	_, err := s.bucketService.GetBucket(req.BucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
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

func (s *Server) GetBucketTagging(w http.ResponseWriter, r *http.Request) {
	req := HandleRequest[BucketRequest](w, r, true, false, nil, nil)
	if req == nil {
		return
	}

	// Implement the logic to get bucket tagging here
	// For now, return an empty Tagging response
	response := Tagging{}

	WriteResponse(w, http.StatusOK, nil, response, nil)
}
