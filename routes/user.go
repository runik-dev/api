package routes

import (
	"fmt"
	"net/http"
	"runik-api/errors"
	"runik-api/nsfw"
	"runik-api/storage"
	"runik-api/structs"
	"runik-api/utils"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func getMe(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}
	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerParseError)
	}
	var user structs.ApiUser
	err = db.Model(&structs.User{}).Where(&structs.User{ID: parsed.UserID}).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		return c.Status(404).JSON(errors.NotFound)
	} else if err != nil {
		return c.Status(500).JSON(errors.ServerSqlError)
	}
	if err != nil {
		return c.Status(500).JSON(errors.ServerStringifyError)
	}

	return c.Status(200).JSON(user)
}

type PutEmail struct {
	Email string `json:"email" validate:"required,email"`
	Url   string `json:"url" validate:"required,url"`
}

func putEmail(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}
	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerParseError)
	}

	var body PutEmail
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}
	avaiable, _ := emailAvailable(body.Email)
	if !avaiable {
		return c.Status(400).JSON(errors.UserEmailTaken)
	}
	token, tokenErr := utils.RandString(32)
	if tokenErr != nil {
		return c.Status(500).JSON(errors.ServerTokenGenerate)
	}
	if err := sendResetEmail(c, body.Email, body.Url, token); err != nil {
		return err
	}
	_, redErr := rdb.Set(ctx, "verification:"+token, parsed.UserID, 30*time.Minute).Result()
	if redErr != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	db.Model(&structs.User{}).Where("ID = ?", parsed.UserID).Updates(map[string]interface{}{"Email": body.Email, "Verified": false})
	return c.Status(http.StatusNoContent).Send(nil)
}

type PutPassword struct {
	OldPassword string `json:"oldPassword" validate:"required,min=8,max=32"`
	NewPassword string `json:"newPassword" validate:"required,min=8,max=32"`
}

func putPassword(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}
	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerParseError)
	}

	var body PutPassword
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}
	var user structs.User
	if err := db.Model(&structs.User{}).Where(&structs.User{ID: parsed.UserID}).Select("password").First(&user).Error; err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.OldPassword)); err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(errors.ServerHash)
	}
	db.Model(&structs.User{}).Where("ID = ?", parsed.UserID).Update("password", hash)
	return c.Status(http.StatusNoContent).Send(nil)
}

type DeleteUser struct {
	Password string `json:"password" validate:"required,min=8,max=32"`
}

func deleteMe(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}
	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerParseError)
	}

	var body DeleteUser
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}

	var user structs.User
	if err := db.Where(&structs.User{ID: parsed.UserID}).First(&user).Error; err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	err = db.Delete(&user).Error
	if err != nil {
		return c.Status(500).JSON(errors.ServerSqlError)
	}
	return c.Status(http.StatusNoContent).Send(nil)
}

type PutAvatar struct {
	Image string `json:"image" validate:"required,base64"`
}

func putAvatar(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}

	var body PutAvatar
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {
		return err
	}

	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerParseError)
	}
	resized, err := storage.Resize(body.Image, 128, 128)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerImageError)
	}
	webp, err := storage.ToWebp(resized)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerImageError)
	}
	isNsfw, err := nsfw.IsNsfw(webp)
	if err != nil {
		fmt.Println(err.Error())
	}
	if isNsfw {
		return c.Status(400).JSON(errors.ImageNsfw)
	}
	fmt.Println("not nsfw")
	err = storage.Upload(parsed.UserID, *webp)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerImageError)
	}
	return c.Status(http.StatusNoContent).Send(nil)
}
func deleteAvatar(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}
	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerParseError)
	}
	if err := storage.Remove(parsed.UserID); err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerStorageError)
	}
	return c.Status(204).Send(nil)
}
