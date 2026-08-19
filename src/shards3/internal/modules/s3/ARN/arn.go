package ARN

import (
	"shards3/internal/platform/config"
)

func getArn(service string, resourceType string, resourceId string) string {
	if resourceType != "" {
		resourceId = ":" + resourceId
	}
	return "arn:aws:" +
		service + ":" +
		config.Cfg.S3Region + ":" +
		config.Cfg.S3AccountID + ":" +
		resourceType +
		resourceId
}

func GetBucketArn(bucketName string) string {
	return getArn("s3", "bucket", bucketName)
}
