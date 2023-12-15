package nsfw

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"

	"github.com/koyachi/go-nude"
)

func IsNsfw(image *bytes.Buffer) (bool, error) {
	tempFile, err := os.CreateTemp("", "nsfwimage-*.jpg")
	if err != nil {
		log.Fatalf("create temp file error " + err.Error())
		return false, err
	}
	fmt.Println(tempFile.Name())
	// defer os.Remove(tempFile.Name())
	png, err := toJpg(image)
	if err != nil {
		log.Fatalf("to png error " + err.Error())
		return false, err
	}
	if _, err := io.Copy(tempFile, png); err != nil {
		log.Fatalf("copy file error " + err.Error())
		return false, err
	}
	if err := tempFile.Close(); err != nil {
		log.Fatalf("close file error " + err.Error())
		return false, err
	}
	result, err := nude.IsNude(tempFile.Name())
	if err != nil {
		fmt.Println("there was am errpr")
		fmt.Println(err.Error())
	}
	fmt.Println(result)
	return result, err
}
func toJpg(img *bytes.Buffer) (*bytes.Buffer, error) {
	var jpgBuffer bytes.Buffer

	imgBytes := img.Bytes()
	imgReader := bytes.NewReader(imgBytes)
	imgDecoded, _, err := image.Decode(imgReader)
	if err != nil {
		return nil, err
	}

	options := &jpeg.Options{Quality: 100}
	if err := jpeg.Encode(&jpgBuffer, imgDecoded, options); err != nil {
		return nil, err
	}

	return &jpgBuffer, nil
}
