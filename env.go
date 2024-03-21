package main

import (
	"log"
	"os"

	"api/structs"
)

func checkEnv() structs.Environment {
	var env structs.Environment

	envVars := []struct {
		name  string
		field *string
	}{
		{"POSTGRES_CONNECTION", &env.PostgresConnection},
		{"API_AUTHENTICATION", &env.ApiAuthentication},

		{"SMTP_HOST", &env.SmtpHost},
		{"SMTP_PORT", &env.SmtpPort},
		{"SMTP_USERNAME", &env.SmtpUsername},
		{"SENDER_EMAIL", &env.SenderEmail},
		{"SENDER_PASSWORD", &env.SenderPassword},

		{"REDIS_ADDRESS", &env.RedisAddress},
		{"REDIS_PASSWORD", &env.RedisPassword},

		{"PORT", &env.Port},
		{"REQUESTS_PER_SECOND", &env.RequestsPerSecond},

		{"GIT_TOKEN", &env.GitToken},
		{"GIT_URL", &env.GitUrl},
		{"GIT_USERNAME", &env.GitUsername},

		{"MINIO_ENDPOINT", &env.MinioEndpoint},
		{"MINIO_ACCESS_KEY_ID", &env.MinioAccessKeyId},
		{"MINIO_ACCESS_KEY", &env.MinioAccessKey},
		{"MINIO_AVATAR_BUCKET", &env.MinioAvatarBucket},
	}

	for _, v := range envVars {
		value, exists := os.LookupEnv(v.name)
		if !exists {
			log.Fatal("Environment variable not found: ", v.name)
		}
		*v.field = value
	}

	return env
}
