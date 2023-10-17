package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/monitor"

	"runik-api/database"
	"runik-api/email"
	"runik-api/routes"
	"runik-api/structs"
)

var ctx = context.Background()

func checkEnv() (structs.Environment, error) {
	var env structs.Environment

	envVars := []struct {
		name  string
		field *string
	}{
		{"CONNECTION_STRING", &env.ConnectionString},
		{"GLOBAL_AUTH", &env.GlobalAuth},
		{"SMTP_HOST", &env.SmtpHost},
		{"SMTP_PORT", &env.SmtpPort},
		{"SENDER_EMAIL", &env.SenderEmail},
		{"SENDER_PASSWORD", &env.SenderPassword},
		{"SMTP_USERNAME", &env.SenderUsername},
		{"REDIS_ADDRESS", &env.RedisAddress},
		{"REDIS_PASSWORD", &env.RedisPassword},
		{"PORT", &env.Port},
		{"RPS", &env.Rps},
	}

	for _, v := range envVars {
		value, exists := os.LookupEnv(v.name)
		if !exists {
			log.Fatal("Environment variable not found: ", v.name)
		}
		*v.field = value
	}

	return env, nil
}

func main() {

	env, err := checkEnv()

	rpsInt, err := strconv.Atoi(env.Rps)
	if err != nil {
		log.Fatal("Failed to convert RPS to integer")
	}
	db := database.Connect(&env)
	rdb := database.RedisConnect(&env)
	sender := email.NewEmailSender(env.SmtpHost, env.SmtpPort, env.SenderUsername, env.SenderPassword, env.SenderEmail)

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
	routes.DefineRoutes(app, db, rdb, &env, sender)

	app.Listen(":" + env.Port)
}
