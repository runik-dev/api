package storage

import (
	"bytes"
	"context"
	"log"

	"api/structs"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	ctx    = context.Background()
	client *minio.Client
	env    *structs.Environment
)

func Connect(_env *structs.Environment) *minio.Client {
	env = _env
	var err error
	client, err = minio.New(env.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(env.MinioAccessKeyId, env.MinioAccessKey, ""),
		Secure: true,
	})
	if err != nil {
		log.Fatalf("Failed to create storage client: %v", err)
	}

	return client
}

func Upload(name string, buffer bytes.Buffer) error {
	_, err := client.PutObject(ctx, env.MinioAvatarBucket, name+".webp", &buffer, int64(buffer.Len()), minio.PutObjectOptions{ContentType: "image/webp"})
	return err
}
func Remove(name string) error {
	return client.RemoveObject(ctx, env.MinioAvatarBucket, name+".webp", minio.RemoveObjectOptions{})
}
