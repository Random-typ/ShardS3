package backup

type Service interface {
	RunBackup(bucket string, prefix string) error
	RestoreBackup(bucket string, objectKey string) error
}
