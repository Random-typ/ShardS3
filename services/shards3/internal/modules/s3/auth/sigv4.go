package auth

import "net/http"

type SigV4 interface {
	VerifyRequest(r *http.Request) error
}
