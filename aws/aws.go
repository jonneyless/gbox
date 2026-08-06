package aws

import (
	"bytes"
	"context"
	"fmt"
	"gbox/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type AWSParams struct {
	AccessKey       string
	AccessKeySecret string
	Region          string
	Bucket          string
}

type AWS struct {
	ctx  context.Context
	log  *zap.SugaredLogger
	cfg  aws.Config
	conf *AWSParams
}

var awsClient *AWS

func GetAWS() *AWS {
	return awsClient
}

func InitAWS(c *AWSParams) {
	log := logger.GetLogger()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(c.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			c.AccessKey,
			c.AccessKeySecret,
			"",
		)),
	)
	if err != nil {
		log.Warnw("Failed to load AWS config:", err)
	}

	awsClient = &AWS{
		cfg:  cfg,
		log:  log,
		conf: c,
	}
}

func (a *AWS) TransferManager() *transfermanager.Client {
	s3Client := s3.NewFromConfig(a.cfg)

	tmClient := transfermanager.New(s3Client, func(o *transfermanager.Options) {
		o.PartSizeBytes = 10 * 1024 * 1024
		o.Concurrency = 3
	})

	return tmClient
}

func (a *AWS) FormatUrl(path string) string {
	if path == "" {
		return ""
	}

	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", a.conf.Bucket, a.conf.Region, path)
}

func (a *AWS) Upload(ctx context.Context, key string, file *bytes.Reader, contentType string) (*transfermanager.UploadObjectOutput, error) {
	client := a.TransferManager()

	result, err := client.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket:      aws.String(a.conf.Bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})

	if err != nil {
		a.log.Warnw("Failed to upload:", "error", err)
	}

	return result, err
}
