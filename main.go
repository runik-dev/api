package main

import (
	"log"
	"strconv"
	"time"

	"api/database"
	"api/email"
	"api/git"
	"api/routes"
	"api/storage"

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
		log.Fatal("Failed to connect to git ", err.Error())
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
