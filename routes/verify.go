package routes

import (
	"net/http"
	"time"

	"api/errors"
	"api/structs"
	"api/utils"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
)

func sendVerifyEmail(c *fiber.Ctx, email string, url string, token string) error {
	err := sender.SendEmail(email, "Verify Email", "Verify your account: "+url+"/"+token)
	if err != nil {
		return c.Status(500).JSON(errors.ServerEmailSend)
	}
	return nil
}

type PostVerifyBody struct {
	Email string `json:"email" validate:"required,email"`
	Url   string `json:"url" validate:"required,url"`
}

func postVerify(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization != env.ApiAuthentication {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	}
	var body PostVerifyBody

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}
	_, user := emailAvailable(body.Email)

	if user.Verified {
		return c.Status(http.StatusBadRequest).JSON(errors.UserAlreadyVerified)
	}
	token, tokenErr := utils.RandString(32)
	if tokenErr != nil {
		return c.Status(500).JSON(errors.ServerTokenGenerate)
	}
	if err := sendVerifyEmail(c, body.Email, body.Url, token); err != nil {
		return err
	}
	_, redErr := rdb.Set(ctx, "verification:"+token, user.ID, 30*time.Minute).Result()
	if redErr != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	return c.Status(http.StatusNoContent).Send(nil)
}
func putVerify(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(400).JSON(errors.MissingParameter)
	}
	id, err := rdb.Get(ctx, "verification:"+token).Result()
	if err == redis.Nil {
		return c.Status(404).JSON(errors.NotFound)
	} else if err != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	db.Model(&structs.User{}).Where("ID = ?", id).Update("verified", true)
	rdb.Del(ctx, "verification:"+token)
	return c.Status(http.StatusNoContent).Send(nil)
}
