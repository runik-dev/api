package routes

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"api/errors"
	"api/structs"
	"api/utils"

	"github.com/bytedance/sonic"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type PostSessions struct {
	Email    string  `json:"email" validate:"required,email"`
	Password string  `json:"password" validate:"required,min=8,max=32"`
	Expire   bool    `json:"expire" validate:"omitempty,boolean"`
	IP       *string `json:"ip" validate:"omitempty,ip"`
}

func getExpiration(expires bool) time.Duration {
	if expires {
		return time.Hour * 24 * 10
	} else {
		return time.Hour * 24 * 30 * 6
	}
}
func getIp(body PostSessions, c *fiber.Ctx) string {
	if body.IP == nil {
		return c.IP()
	} else {
		return *body.IP
	}
}
func postSessions(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization != env.ApiAuthentication {
		return c.Status(http.StatusUnauthorized).JSON(errors.AuthorizationInvalid)
	}

	var body PostSessions
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {
		return err
	}

	var user structs.User
	if err := db.Where(&structs.User{Email: body.Email}).First(&user).Error; err != nil {
		fmt.Println("User not found")
		return c.Status(http.StatusNotFound).JSON(errors.UserCredentialsInvalid)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		fmt.Println("password wrong")
		return c.Status(http.StatusUnauthorized).JSON(errors.UserCredentialsInvalid)
	}
	if user.TotpVerified {
		id := generator.Generate()
		rdb.Set(ctx, "totp:"+id.String(), user.ID, time.Minute*15)
		return c.Status(http.StatusOK).JSON(fiber.Map{"action_name": "totp", "totp_id": id.String()})
	}
	ip := getIp(body, c)
	expiration := getExpiration(body.Expire)
	token, err := utils.RandString(32)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(errors.ServerTokenGenerate)
	}
	stringified, err := sonic.Marshal(structs.Session{UserID: user.ID, IP: ip})
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerStringifyError)
	}
	rdb.Set(ctx, "session:"+token, stringified, expiration)
	return c.JSON(fiber.Map{"token": token})
}
func confirm2faSignIn(c *fiber.Ctx) error {
	totpId := c.Params("totp")
	if totpId == "" {
		return c.Status(http.StatusBadRequest).JSON(errors.MissingParameter)
	}

	expireStr := c.Query("expire")
	var expire bool
	if expireStr == "true" {
		expire = true
	}

	authorization := c.Get("Authorization")
	if authorization != env.ApiAuthentication {
		return c.Status(http.StatusUnauthorized).JSON(errors.AuthorizationInvalid)
	}

	code := c.Query("code")
	if code == "" {
		return c.Status(http.StatusBadRequest).JSON(errors.MissingParameter)
	}

	val, err := rdb.Get(ctx, "totp:"+totpId).Result()
	if err == redis.Nil {
		return c.Status(http.StatusUnauthorized).JSON(errors.NotFound)
	} else if err != nil {
		return c.Status(500).JSON(errors.ServerSqlError)
	}
	var user structs.User
	if err := db.Where(&structs.User{ID: val}).First(&user).Error; err != nil {
		fmt.Println("User not found")
		return c.Status(http.StatusNotFound).JSON(errors.UserCredentialsInvalid)
	}
	valid := totp.Validate(code, user.TotpSecret)
	if !valid {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"code": "invalid_totp"})
	}

	ip := c.IP() // add IP optionin body at somepoint
	expiration := getExpiration(expire)
	token, err := utils.RandString(32)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(errors.ServerTokenGenerate)
	}
	stringified, err := sonic.Marshal(structs.Session{UserID: user.ID, IP: ip})
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerStringifyError)
	}
	rdb.Set(ctx, "session:"+token, stringified, expiration)
	rdb.Del(ctx, "totp:"+totpId)
	return c.JSON(fiber.Map{"token": token})
}
func getSessionIps(c *fiber.Ctx, userId string) ([]string, error) {
	iter := rdb.Scan(ctx, 0, "session:*", 0).Iterator()
	var sessions []string
	for iter.Next(ctx) {
		key := iter.Val()
		val, err := rdb.Get(ctx, key).Result()
		if err != nil {
			fmt.Println(err.Error())
			return nil, c.Status(http.StatusInternalServerError).JSON(errors.ServerRedisError)
		}
		var parsed structs.Session
		err = sonic.UnmarshalString(val, &parsed)
		if err != nil {
			fmt.Println(err.Error())
			return nil, c.Status(http.StatusInternalServerError).JSON(errors.ServerParseError)
		}
		if parsed.UserID == userId {
			sessions = append(sessions, parsed.IP)
		}
	}
	if err := iter.Err(); err != nil {
		fmt.Println(err.Error())
		return nil, c.Status(http.StatusInternalServerError).JSON(errors.ServerRedisError)
	}
	return sessions, nil
}
func getSessions(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(http.StatusUnauthorized).JSON(errors.AuthorizationMissing)
	}
	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(http.StatusUnauthorized).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerParseError)
	}
	sessions, err := getSessionIps(c, parsed.UserID)
	if err != nil {
		return err
	}
	return c.Status(http.StatusOK).JSON(sessions)
}

type DeleteSessions struct {
	Password string `json:"password" validate:"required,min=8,max=32"`
}

func deleteSessionEntries(parsed structs.Session, c *fiber.Ctx) ([]string, error) {
	keys, _, err := rdb.Scan(ctx, 0, "session:"+"*", 0).Result()
	if err != nil {
		return nil, c.Status(http.StatusInternalServerError).JSON(errors.ServerRedisError)
	}
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			session, err := rdb.Get(ctx, key).Result()
			if err != nil {
				return
			}
			var parsedSession structs.Session
			err = sonic.UnmarshalString(session, &parsedSession)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			if parsedSession.UserID == parsed.UserID {
				_, err = rdb.Del(ctx, key).Result()
				if err != nil {
					return
				}
			}
		}(key)
	}
	wg.Wait()
	return keys, nil
}
func deleteSessions(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization == "" {
		return c.Status(http.StatusUnauthorized).JSON(errors.AuthorizationMissing)
	}
	var body DeleteSessions
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {

		return err
	}

	session, err := rdb.Get(ctx, "session:"+authorization).Result()
	if err == redis.Nil {
		return c.Status(http.StatusUnauthorized).JSON(errors.AuthorizationInvalid)
	} else if err != nil {
		fmt.Println(err.Error())
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerRedisError)
	}
	var parsed structs.Session
	err = sonic.UnmarshalString(session, &parsed)
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerParseError)
	}
	var user structs.User
	if err := db.Model(&structs.User{}).Where(&structs.User{ID: parsed.UserID}).Select("password").First(&user).Error; err != nil {
		return c.Status(http.StatusBadRequest).JSON(errors.UserCredentialsInvalid)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		return c.Status(http.StatusBadRequest).JSON(errors.UserCredentialsInvalid)
	}
	keys, err := deleteSessionEntries(parsed, c)
	if err != nil {
		return err
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"deleted": len(keys)})
}
func deleteSession(c *fiber.Ctx) error {
	authorization := c.Params("token")
	if authorization == "" {
		return c.Status(http.StatusUnauthorized).JSON(errors.AuthorizationMissing)
	}
	s, err := rdb.Del(ctx, "session:"+authorization).Result()
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerRedisError)
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"success": s != 0})
}
