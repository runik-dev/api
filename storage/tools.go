package storage

import (
	"bytes"
	"encoding/base64"
	"image"
	"strings"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
)

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
