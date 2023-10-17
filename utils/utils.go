package utils

import (
	"crypto/rand"
	"encoding/hex"
)

func RandString(length int) (string, error) {
	randomBytes := make([]byte, length/2) // Divide by 2 because 2 hex characters represent each byte
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	randomString := hex.EncodeToString(randomBytes)
	return randomString, nil
}
