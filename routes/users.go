package routes

import (
	"fmt"
	"time"

	"api/errors"
	"api/structs"
	"api/utils"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type PostBody struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=32"`
	Url      string `json:"url" validate:"required,url"`
}

func postUsers(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization != env.ApiAuthentication {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	}
	var body PostBody

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}
	available, _ := emailAvailable(body.Email)
	if !available {
		return c.Status(400).JSON(errors.UserEmailTaken)
	}
	token, tokenErr := utils.RandString(32)
	if tokenErr != nil {
		return c.Status(500).JSON(errors.ServerTokenGenerate)
	}
	if err := sendVerifyEmail(c, body.Email, body.Url, token); err != nil {
		return err
	}
	id := generator.Generate()
	_, redErr := rdb.Set(ctx, "verification:"+token, id.String(), 30*time.Minute).Result()
	if redErr != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(errors.ServerHash)
	}
	user := structs.User{ID: id.String(), Email: body.Email, Password: string(hash)}
	err = db.Create(&user).Error
	if err == gorm.ErrDuplicatedKey {
		return c.Status(400).JSON(errors.UserEmailTaken)
	} else if err != nil {
		fmt.Println(err)
		return c.Status(500).JSON(errors.ServerSqlError)
	}
	return c.Status(200).JSON(fiber.Map{"id": id.String()})
}
func getUsers(c *fiber.Ctx) error {
	var users []structs.ApiUser
	val, err := rdb.Get(ctx, "users").Result()
	if err != nil {
		db.Model(&structs.User{}).Find(&users)

		json, err := sonic.Marshal(&users)
		if err == nil {
			rdb.Set(ctx, "users", json, 1*time.Minute)
		}
	} else {
		err := sonic.Unmarshal([]byte(val), &users)
		if err != nil {
			return c.Status(500).JSON(errors.ServerParseError)
		}
	}

	return c.Status(200).JSON(users)
}
