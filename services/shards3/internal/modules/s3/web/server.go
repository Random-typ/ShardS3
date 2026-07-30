package web

import (
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/hostrouter"

	"shards3/services/shards3/internal/modules/s3/auth"
	"shards3/services/shards3/internal/modules/s3/bucket"
	"shards3/services/shards3/internal/platform/config"
)

type Server struct {
	bucketService bucket.Service
	authService   auth.SigV4
}

func NewServer(bucketService bucket.Service, authService auth.SigV4) *Server {
	return &Server{bucketService: bucketService, authService: authService}
}

func (s *Server) Routes() http.Handler {
	rootHostRouter := chi.NewRouter()
	rootHostRouter.Get("/*", s.handleBucketsGet)

	bucketHostRouter := chi.NewRouter()
	bucketHostRouter.Get("/*", s.handleBucketPresentGet)
	bucketHostRouter.Head("/*", s.handleBucketPresentHead)
	bucketHostRouter.Put("/*", s.handleBucketPresentPut)
	bucketHostRouter.Post("/*", s.handleBucketPresentPost)
	bucketHostRouter.Delete("/*", s.handleBucketPresentDelete)

	hosts := hostrouter.New()
	hosts.Map(config.Cfg.FQDN, rootHostRouter)
	hosts.Map("*."+config.Cfg.FQDN, bucketHostRouter)

	return s.logRequests(s.requireSigV4(hosts))
}

func normalizeHost(host string) string {
	h := strings.TrimSpace(strings.ToLower(host))
	if parsedHost, _, err := net.SplitHostPort(h); err == nil {
		h = parsedHost
	}
	return strings.TrimSuffix(h, ".")
}

func bucketFromHost(host, fqdn string) (string, bool) {
	normalizedHost := normalizeHost(host)
	normalizedFQDN := normalizeHost(fqdn)
	if normalizedHost == "" || normalizedFQDN == "" || normalizedHost == normalizedFQDN {
		return "", false
	}

	suffix := "." + normalizedFQDN
	if !strings.HasSuffix(normalizedHost, suffix) {
		return "", false
	}

	bucketName := strings.TrimSuffix(normalizedHost, suffix)
	if bucketName == "" || strings.Contains(bucketName, ".") {
		return "", false
	}

	return bucketName, true
}
