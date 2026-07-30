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
	Xmlns   string   `xml:"xmlns,attr"`

	Buckets           []Bucket `xml:"Buckets>Bucket,omitempty"`
	Owner             Owner    `xml:"Owner,omitempty"`
	ContinuationToken string   `xml:"ContinuationToken,omitempty"`
	Prefix            string   `xml:"Prefix,omitempty"`
}

type Owner struct {
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

	Xmlns              string `xml:"xmlns,attr"`
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
	Xmlns   string   `xml:"xmlns,attr"`
	Rules   []Rule   `xml:"Rule,omitempty"`
}

type Grantee struct {
	XMLName xml.Name `xml:"Grantee"`

	Xmlns        string `xml:"xmlns,attr"`
	DisplayName  string `xml:"DisplayName,omitempty"`
	EmailAddress string `xml:"EmailAddress,omitempty"`
	ID           string `xml:"ID,omitempty"`
	Type         string `xml:"xsi:type,attr,omitempty"`
	URI          string `xml:"URI,omitempty"`
}

type AccessControlList struct {
	XMLName xml.Name `xml:"AccessControlList"`

	Xmlns      string    `xml:"xmlns,attr"`
	Grant      []Grantee `xml:"Grant>Grantee,omitempty"`
	Permission string    `xml:"Permission,omitempty"`
}

type AccessControlPolicy struct {
	XMLName xml.Name `xml:"AccessControlPolicy"`

	Xmlns             string              `xml:"xmlns,attr"`
	Owner             Owner               `xml:"Owner,omitempty"`
	AccessControlList []AccessControlList `xml:"AccessControlList>Grant,omitempty"`
}

type Tag struct {
	XMLName xml.Name `xml:"Tag"`

	Key   string `xml:"Key,omitempty"`
	Value string `xml:"Value,omitempty"`
}

type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`

	Xmlns  string `xml:"xmlns,attr"`
	TagSet []Tag  `xml:"TagSet>Tag,omitempty"`
}

func urlEncode(s string) string {
	return url.QueryEscape(s)
}

func (s *Server) GetBucketLocation(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket name not found in host", r.URL.Path)
		return
	}

	_, err := s.bucketService.GetBucket(bucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}

	response := LocationConstraint{
		Xmlns:              "http://s3.amazonaws.com/doc/2006-03-01/",
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
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket name not found in host", r.URL.Path)
		return
	}

	_, err := s.bucketService.GetBucket(bucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("x-amz-bucket-arn", ARN.GetBucketArn(bucketName))
	w.Header().Set("x-amz-bucket-region", config.Cfg.S3Region)
}

func (s *Server) ListBuckets(w http.ResponseWriter, r *http.Request) {
	continuationToken, _ := strconv.Atoi(r.URL.Query().Get("continuation-token"))
	prefix := r.URL.Query().Get("prefix")
	maxBuckets, _ := strconv.Atoi(r.URL.Query().Get("max-buckets"))
	if continuationToken <= 0 {
		continuationToken = 1
	}
	if maxBuckets <= 0 || maxBuckets > 10000 {
		maxBuckets = 10000
	}

	buckets, hasMore, err := s.bucketService.ListBuckets(prefix, continuationToken, maxBuckets)
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", err.Error(), r.URL.Path)
		return
	}
	if hasMore {
		continuationToken++
	} else {
		continuationToken = 0
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
		Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/",
		Owner: Owner{
			ID:          config.Cfg.S3AccountID,
			DisplayName: config.Cfg.ServiceName,
		},
		Buckets: items,
		Prefix:  prefix,
	}

	if continuationToken != 0 {
		response.ContinuationToken = strconv.Itoa(continuationToken)
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

func (s *Server) GetBucketEncryption(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket name not found in host", r.URL.Path)
		return
	}

	_, err := s.bucketService.GetBucket(bucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}
	encryption := ServerSideEncryptionConfiguration{
		Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/",
		Rules: []Rule{
			{
				SSEAlgorithm:     config.Cfg.EncryptionMethod,
				BucketKeyEnabled: false,
			},
		},
	}

	payload, err := xml.MarshalIndent(encryption, "", "  ")
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}

func (s *Server) GetBucketAcl(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket name not found in host", r.URL.Path)
		return
	}

	_, err := s.bucketService.GetBucket(bucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}
	accessControlPolicy := AccessControlPolicy{
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

	payload, err := xml.MarshalIndent(accessControlPolicy, "", "  ")
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}

func (s *Server) GetBucketTagging(w http.ResponseWriter, r *http.Request) {
	bucketName, ok := bucketFromHost(r.Host, config.Cfg.FQDN)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidBucketName", "bucket name not found in host", r.URL.Path)
		return
	}
	_, err := s.bucketService.GetBucket(bucketName)
	if err != nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found", r.URL.Path)
		return
	}
	// Implement the logic to get bucket tagging here
	// For now, return an empty Tagging response
	tagging := Tagging{
		Xmlns:  "http://s3.amazonaws.com/doc/2006-03-01/",
		TagSet: []Tag{},
	}

	payload, err := xml.MarshalIndent(tagging, "", "  ")
	if err != nil {
		writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to encode response", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(payload)
}
