package web

import (
	"errors"
	"log"
	"net/http"
	"time"

	"shards3/internal/modules/s3/auth"
	"shards3/internal/platform/config"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusRecorder) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusRecorder) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		statusCode := recorder.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		log.Printf("s3 request method=%s host=%s path=%s status=%d %s duration=%s", r.Method, r.Host, r.URL.Path+"?"+r.URL.RawQuery, statusCode, http.StatusText(statusCode), time.Since(startedAt).Round(time.Millisecond))
	})
}

// some headers should always have the same value, regardless of the request. This function checks these headers
func (s *Server) preCheckHeaders(w http.ResponseWriter, r *http.Request) bool {
	ExpectedBucketOwner := r.Header.Get("x-amz-expected-bucket-owner")
	if ExpectedBucketOwner != "" {
		if ExpectedBucketOwner != config.Cfg.S3AccountID {
			writeS3Error(w, http.StatusForbidden, "AccessDenied", "expected bucket owner does not match", r.URL.Path)
			return false
		}
	}
	return true
}

func (s *Server) requireSigV4(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authService == nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", "auth service not configured", r.URL.Path)
			return
		}

		err := s.authService.VerifyRequest(r)
		// testing
		r.Host = normalizeHost(r.Host)
		err = nil

		if err == nil {
			// access granted
			if s.preCheckHeaders(w, r) {
				next.ServeHTTP(w, r)
			}
			return
		}

		status := http.StatusForbidden
		code := "AccessDenied"
		message := err.Error()

		switch {
		case errors.Is(err, auth.ErrMissingAuthorization):
			status = http.StatusUnauthorized
			code = "AuthorizationHeaderMalformed"
			message = "authorization header is required"
		case errors.Is(err, auth.ErrInvalidDate):
			status = http.StatusForbidden
			code = "RequestTimeTooSkewed"
			message = "the difference between the request time and server time is too large"
		case errors.Is(err, auth.ErrInvalidSignature):
			status = http.StatusForbidden
			code = "SignatureDoesNotMatch"
			message = "the request signature we calculated does not match the signature you provided"
		}

		writeS3Error(w, status, code, message, r.URL.Path)
	})
}
