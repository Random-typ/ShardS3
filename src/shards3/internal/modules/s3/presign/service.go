package presign

type Service interface {
	CreatePresignedURL(bucket string, key string, method string, expiresSeconds int64) (string, error)
}
