package auth

type SigV4 interface {
	VerifyAuthorizationHeader(authHeader string) error
}
