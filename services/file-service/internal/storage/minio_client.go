package storage

import (
	"bytes"
	"context"
	"io"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const bucketName = "dropbox-poc"

type MinioClient struct {
	client *minio.Client
}

func NewMinioClient(endpoint, accessKey, secretKey string) (*MinioClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
		log.Printf("minio: created bucket %q", bucketName)
	}

	return &MinioClient{client: client}, nil
}

func (m *MinioClient) Put(ctx context.Context, key string, data []byte) error {
	_, err := m.client.PutObject(ctx, bucketName, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	return err
}

func (m *MinioClient) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := m.client.GetObject(ctx, bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}
