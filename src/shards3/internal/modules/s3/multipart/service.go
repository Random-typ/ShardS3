package multipart

type Service interface {
	CreateUpload(bucket string, key string) error
	UploadPart(bucket string, uploadID string, partNumber int) error
	CompleteUpload(bucket string, uploadID string) error
	AbortUpload(bucket string, uploadID string) error
}
