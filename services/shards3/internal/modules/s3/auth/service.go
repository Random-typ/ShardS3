package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	sigV4Algorithm  = "AWS4-HMAC-SHA256"
	emptySHA256Hash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	s3ServiceName   = "s3"
)

var (
	ErrMissingAuthorization = errors.New("missing authorization header")
	ErrInvalidAuthorization = errors.New("invalid authorization header")
	ErrInvalidSignature     = errors.New("invalid signature")
	ErrInvalidDate          = errors.New("invalid or expired x-amz-date")
)

type Config struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	AllowedSkew     time.Duration
}

type Service struct {
	accessKeyID     string
	secretAccessKey string
	region          string
	allowedSkew     time.Duration
	now             func() time.Time
}

func NewService(cfg Config) *Service {
	allowedSkew := cfg.AllowedSkew
	if allowedSkew <= 0 {
		allowedSkew = 5 * time.Minute
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	return &Service{
		accessKeyID:     cfg.AccessKeyID,
		secretAccessKey: cfg.SecretAccessKey,
		region:          region,
		allowedSkew:     allowedSkew,
		now:             time.Now,
	}
}

func (s *Service) VerifyRequest(r *http.Request) error {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return ErrMissingAuthorization
	}

	if !strings.HasPrefix(authHeader, sigV4Algorithm+" ") {
		return fmt.Errorf("%w: unsupported authorization algorithm", ErrInvalidAuthorization)
	}

	fields, err := parseAuthorizationFields(strings.TrimPrefix(authHeader, sigV4Algorithm+" "))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAuthorization, err)
	}

	credential, ok := fields["Credential"]
	if !ok {
		return fmt.Errorf("%w: missing credential", ErrInvalidAuthorization)
	}
	signedHeaders, ok := fields["SignedHeaders"]
	if !ok || signedHeaders == "" {
		return fmt.Errorf("%w: missing signed headers", ErrInvalidAuthorization)
	}
	signature, ok := fields["Signature"]
	if !ok || signature == "" {
		return fmt.Errorf("%w: missing signature", ErrInvalidAuthorization)
	}

	credParts := strings.Split(credential, "/")
	if len(credParts) != 5 {
		return fmt.Errorf("%w: malformed credential scope", ErrInvalidAuthorization)
	}
	accessKeyID, scopeDate, scopeRegion, scopeService, scopeTerminator := credParts[0], credParts[1], credParts[2], credParts[3], credParts[4]
	if accessKeyID != s.accessKeyID {
		return fmt.Errorf("%w: access key mismatch", ErrInvalidAuthorization)
	}
	if scopeRegion != s.region || scopeService != s3ServiceName || scopeTerminator != "aws4_request" {
		return fmt.Errorf("%w: invalid credential scope", ErrInvalidAuthorization)
	}

	amzDate := strings.TrimSpace(r.Header.Get("X-Amz-Date"))
	if amzDate == "" {
		return fmt.Errorf("%w: missing x-amz-date", ErrInvalidDate)
	}
	signedAt, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		return fmt.Errorf("%w: malformed x-amz-date", ErrInvalidDate)
	}
	now := s.now().UTC()
	if signedAt.Sub(now) > s.allowedSkew || now.Sub(signedAt) > s.allowedSkew {
		return ErrInvalidDate
	}
	if signedAt.Format("20060102") != scopeDate {
		return fmt.Errorf("%w: x-amz-date and scope date mismatch", ErrInvalidAuthorization)
	}

	payloadHash := strings.TrimSpace(r.Header.Get("X-Amz-Content-Sha256"))
	if payloadHash == "" {
		if r.ContentLength == 0 {
			payloadHash = emptySHA256Hash
		} else {
			return fmt.Errorf("%w: missing x-amz-content-sha256", ErrInvalidAuthorization)
		}
	}

	signedHeaderNames := strings.Split(signedHeaders, ";")
	if len(signedHeaderNames) == 0 {
		return fmt.Errorf("%w: empty signed headers", ErrInvalidAuthorization)
	}
	if !isSortedLowerHeaderList(signedHeaderNames) {
		return fmt.Errorf("%w: signed headers must be lowercase and sorted", ErrInvalidAuthorization)
	}

	canonicalHeaders, err := buildCanonicalHeaders(r, signedHeaderNames)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAuthorization, err)
	}

	canonicalRequest := strings.Join([]string{
		r.Method,
		canonicalURI(r.URL),
		canonicalQueryString(r.URL),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	canonicalRequestHash := sha256.Sum256([]byte(canonicalRequest))
	scope := strings.Join([]string{scopeDate, scopeRegion, s3ServiceName, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		scope,
		hex.EncodeToString(canonicalRequestHash[:]),
	}, "\n")

	signingKey := deriveSigningKey(s.secretAccessKey, scopeDate, scopeRegion, s3ServiceName)
	expectedSigBytes := hmacSHA256(signingKey, []byte(stringToSign))
	expectedSig := hex.EncodeToString(expectedSigBytes)

	if subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSig)) != 1 {
		return ErrInvalidSignature
	}

	return nil
}

func parseAuthorizationFields(raw string) (map[string]string, error) {
	parts := strings.Split(raw, ",")
	fields := make(map[string]string, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("malformed field %q", part)
		}
		fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return fields, nil
}

func isSortedLowerHeaderList(headers []string) bool {
	if len(headers) == 0 {
		return false
	}
	for i := range headers {
		if headers[i] == "" || headers[i] != strings.ToLower(headers[i]) {
			return false
		}
		if i > 0 && headers[i-1] > headers[i] {
			return false
		}
	}
	return true
}

func buildCanonicalHeaders(r *http.Request, signedHeaderNames []string) (string, error) {
	var lines []string
	for _, name := range signedHeaderNames {
		var value string
		switch name {
		case "host":
			value = r.Host
		default:
			value = r.Header.Get(name)
		}
		value = strings.TrimSpace(collapseSpaces(value))
		if value == "" {
			return "", fmt.Errorf("missing signed header %q", name)
		}
		lines = append(lines, name+":"+value)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func canonicalURI(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQueryString(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}
	values, _ := url.ParseQuery(u.RawQuery)
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0)
	for _, k := range keys {
		vals := values[k]
		sort.Strings(vals)
		if len(vals) == 0 {
			pairs = append(pairs, awsQueryEscape(k)+"=")
			continue
		}
		for _, v := range vals {
			pairs = append(pairs, awsQueryEscape(k)+"="+awsQueryEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}

func awsQueryEscape(s string) string {
	escaped := url.QueryEscape(s)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
