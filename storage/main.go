package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"log"
	"strings"

	"runik-api/structs"

	"cloud.google.com/go/storage"
	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
)

var (
	ctx    = context.Background()
	bucket *storage.BucketHandle
)

func Connect(env *structs.Environment) (*storage.Client, *storage.BucketHandle) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create storage client: %v", err)
	}
	bucket = client.Bucket(env.StorageBucket)
	return client, bucket
}

func Resize(b64 string, w int, h int) (*image.NRGBA, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	resized := imaging.Resize(img, w, h, imaging.Lanczos)
	return resized, nil
}
func ToWebp(img *image.NRGBA) (*bytes.Buffer, error) {
	var buffer bytes.Buffer

	if err := webp.Encode(&buffer, img, &webp.Options{Lossless: true}); err != nil {
		return nil, err
	}
	return &buffer, nil
}
func Upload(name string, buffer bytes.Buffer) error {
	writer := bucket.Object("avatars/" + name + ".webp").NewWriter(ctx)
	defer writer.Close()

	_, err := writer.Write(buffer.Bytes())
	if err != nil {
		return err
	}
	return nil
}
func Remove(name string) error {
	err := bucket.Object("avatars/" + name + ".webp").Delete(ctx)
	return err
}
