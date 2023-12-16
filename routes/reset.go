package routes

import (
	"net/http"
	"runik-api/errors"
	"runik-api/structs"
	"runik-api/utils"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

func sendResetEmail(c *fiber.Ctx, email string, url string, token string) error {
	err := sender.SendEmail(email, "Reset password", "Reset your password: "+url+"/"+token)
	if err != nil {
		return c.Status(500).JSON(errors.ServerEmailSend)
	}
	return nil
}

type PostResetBody struct {
	Email string `json:"email" validate:"required,email"`
	Url   string `json:"url" validate:"required,url"`
}

func postReset(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization != env.GlobalAuth {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	}
	var body PostResetBody

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}
	available, user := emailAvailable(body.Email)
	if available {
		return c.Status(400).JSON(errors.NotFound)
	}
	token, tokenErr := utils.RandString(32)
	if tokenErr != nil {
		return c.Status(500).JSON(errors.ServerTokenGenerate)
	}
	if err := sendResetEmail(c, body.Email, body.Url, token); err != nil {
		return err
	}

	_, redErr := rdb.Set(ctx, "reset:"+token, user.ID, 30*time.Minute).Result()
	if redErr != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	return c.Status(http.StatusNoContent).Send(nil)
}

type PutReset struct {
	Password string `json:"password" validate:"required,min=8,max=32"`
}

func putReset(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(400).JSON(errors.MissingParameter)
	}
	id, err := rdb.Get(ctx, "reset:"+token).Result()
	if err == redis.Nil {
		return c.Status(404).JSON(errors.NotFound)
	} else if err != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	var body PutReset
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}
	rdb.Del(ctx, "reset:"+token)
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(errors.ServerHash)
	}
	db.Model(&structs.User{}).Where("ID = ?", id).Update("password", hash)
	return c.Status(http.StatusNoContent).Send(nil)
}
