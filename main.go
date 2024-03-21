package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"api/database"
	"api/email"
	"api/git"
	"api/routes"
	"api/storage"
	"api/structs"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	env := checkEnv()

	rpsInt, err := strconv.Atoi(env.RequestsPerSecond)
	if err != nil {
		log.Fatal("Failed to convert RPS to integer")
	}

	storage.Connect(&env)

	db := database.Connect(&env)

	rdb := database.RedisConnect(&env)
	sender := email.NewEmailSender(env.SmtpHost, env.SmtpPort, env.SmtpUsername, env.SenderPassword, env.SenderEmail)

	git, err := git.Connect(&env)
	if err != nil {
		log.Fatal("Failed to connect to git", err.Error())
	}

	app := fiber.New(fiber.Config{
		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,
		Prefork:     true,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusBadRequest).JSON(routes.GlobalErrorHandlerResp{
				Success: false,
				Message: err.Error(),
			})
		},
	})
	app.Use(logger.New())
	app.Use(limiter.New(limiter.Config{
		Max:        rpsInt,
		Expiration: 1 * time.Second,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"code": "too_many_requests"})
		},
	}))
	app.Use(compress.New(compress.Config{
		Level: 1,
	}))
	app.Get("/monitor", monitor.New())
	// router.Use(middleware.LeakBucket(limiter))
	routes.DefineRoutes(app, db, rdb, &env, sender, git)

	app.Listen(":" + env.Port)
}
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

		{"STORAGE_BUCKET", &env.StorageBucket},

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
