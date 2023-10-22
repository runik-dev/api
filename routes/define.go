package routes

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"runik-api/email"
	"runik-api/errors"
	"runik-api/structs"
	"runik-api/utils"

	"github.com/bwmarrin/snowflake"
	"github.com/bytedance/sonic"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	db        *gorm.DB
	rdb       *redis.Client
	env       *structs.Environment
	sender    *email.EmailSender
	ctx       = context.Background()
	generator *snowflake.Node
	validate  = validator.New()
	_validate *XValidator
)

type (
	ErrorResponse struct {
		Error       bool
		FailedField string
		Tag         string
		Value       interface{}
	}

	XValidator struct {
		validator *validator.Validate
	}

	GlobalErrorHandlerResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
)

func DefineRoutes(r *fiber.App, database *gorm.DB, redisDatabase *redis.Client, environment *structs.Environment, emailSender *email.EmailSender) {
	db = database
	rdb = redisDatabase
	env = environment
	sender = emailSender
	snowflake.Epoch = 1697015375
	node, err := snowflake.NewNode(1)
	if err != nil {
		log.Fatal("failed to create snowflake node " + err.Error())
		return
	}
	generator = node

	_validate = &XValidator{
		validator: validate,
	}

	v1 := r.Group("/api/v1")
	users := v1.Group("/users")

	users.Post("/", postUsers)
	users.Get("/", getUsers)

	users.Post("/sessions", postSessions)
	users.Delete("/sessions", deleteSessions)
	users.Get("/sessions", getSessions)
	users.Delete("/sessions/:token", deleteSession)

	users.Get("/me", getMe)
	users.Put("/me/email", putEmail)
	users.Put("/me/password", putPassword)
	users.Delete("/me", deleteMe)

	users.Post("/verify", postVerify)
	users.Put("/verify/:token", putVerify)

	users.Post("/reset", postReset)
	users.Put("/reset/:token", putReset)
}

func emailAvailable(email string) (bool, structs.User) {
	var user structs.User
	found := db.Where("email = ?", email).First(&user).Error
	return found == gorm.ErrRecordNotFound, user
}

func handleValidateErrors(errs []ErrorResponse, c *fiber.Ctx) error {
	if len(errs) > 0 && errs[0].Error {
		errMsgs := make([]string, 0)

		for _, err := range errs {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"[%s]: '%v' | Needs to implement '%s'",
				err.FailedField,
				err.Value,
				err.Tag,
			))
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": errMsgs, "code": "malformed_body"})
	}
	return nil
}

func (v XValidator) Validate(data interface{}) []ErrorResponse {
	validationErrors := []ErrorResponse{}

	errs := validate.Struct(data)
	if errs != nil {
		for _, err := range errs.(validator.ValidationErrors) {
			// In this case data object is actually holding the User struct
			var elem ErrorResponse

			elem.FailedField = err.Field() // Export struct field name
			elem.Tag = err.Tag()           // Export struct tag
			elem.Value = err.Value()       // Export field value
			elem.Error = true

			validationErrors = append(validationErrors, elem)
		}
	}

	return validationErrors
}

func sendResetEmail(c *fiber.Ctx, email string, url string, token string) error {
	err := sender.SendEmail(email, "Reset password", "Reset your password: "+url+"/"+token)
	if err != nil {
		return c.Status(500).JSON(errors.ServerEmailSend)
	}
	return nil
}
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
	if authorization != env.GlobalAuth {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	}
	var body PostVerifyBody

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err := handleValidateErrors(errs, c); err != nil {
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
	if err := sendResetEmail(c, body.Email, body.Url, token); err != nil {
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

type PostBody struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=32"`
	Url      string `json:"url" validate:"required,url"`
}

func postUsers(c *fiber.Ctx) error {
	authorization := c.Get("Authorization")
	if authorization != env.GlobalAuth {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	}
	var body PostBody

	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err := handleValidateErrors(errs, c); err != nil {
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
	if err := sendResetEmail(c, body.Email, body.Url, token); err != nil {
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
	if err == redis.Nil {
		db.Model(&structs.User{}).Find(&users)
		usersJSON, err := sonic.Marshal(&users)
		if err != nil {
			return c.Status(500).JSON(errors.ServerStringifyError)
		}

		err = rdb.Set(ctx, "users", usersJSON, 1*time.Minute).Err()
		if err != nil {
			return c.Status(500).JSON(errors.ServerRedisError)
		}
	} else {
		err := sonic.Unmarshal([]byte(val), &users)
		if err != nil {
			return c.Status(500).JSON(errors.ServerParseError)
		}
	}

	return c.Status(200).JSON(users)
}
func getSessionIps(c *fiber.Ctx, userId string) ([]string, error) {
	iter := rdb.Scan(ctx, 0, "session:*", 0).Iterator()
	var sessions []string
	for iter.Next(ctx) {
		key := iter.Val()
		val, err := rdb.Get(ctx, key).Result()
		if err != nil {
			fmt.Println(err.Error())
			return nil, c.Status(500).JSON(errors.ServerRedisError)
		}
		var parsed structs.Session
		err = sonic.UnmarshalString(val, &parsed)
		if err != nil {
			fmt.Println(err.Error())
			return nil, c.Status(500).JSON(errors.ServerParseError)
		}
		if parsed.UserID == userId {
			sessions = append(sessions, parsed.IP)
		}
	}
	if err := iter.Err(); err != nil {
		fmt.Println(err.Error())
		return nil, c.Status(500).JSON(errors.ServerRedisError)
	}
	return sessions, nil
}
func getSessions(c *fiber.Ctx) error {
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
	sessions, err := getSessionIps(c, parsed.UserID)
	if err != nil {
		return err
	}
	return c.Status(200).JSON(sessions)
}

type PostSessions struct {
	Email    string  `json:"email" validate:"required,email"`
	Password string  `json:"password" validate:"required,min=8,max=32"`
	Expire   bool    `json:"expire" validate:"omitempty,boolean"`
	IP       *string `json:"ip" validate:"omitempty,ip"`
}

func getExpiration(body PostSessions) time.Duration {
	if body.Expire {
		return time.Hour * 24 * 10
	} else {
		return -1
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
	if authorization != env.GlobalAuth {
		return c.Status(401).JSON(errors.AuthorizationInvalid)
	}

	var body PostSessions
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err := handleValidateErrors(errs, c); err != nil {
		return err
	}

	var user structs.User
	if err := db.Where(&structs.User{Email: body.Email}).First(&user).Error; err != nil {
		fmt.Println("User not found")
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		fmt.Println("password wrong")
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	ip := getIp(body, c)
	expiration := getExpiration(body)
	token, err := utils.RandString(32)
	if err != nil {
		return c.Status(400).JSON(errors.ServerTokenGenerate)
	}
	stringified, err := sonic.Marshal(structs.Session{UserID: user.ID, IP: ip})
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(500).JSON(errors.ServerStringifyError)
	}
	rdb.Set(ctx, "session:"+token, stringified, expiration)
	return c.JSON(fiber.Map{"token": token})
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
	if err := handleValidateErrors(errs, c); err != nil {
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

func deleteSession(c *fiber.Ctx) error {
	authorization := c.Params("token")
	if authorization == "" {
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}
	s, err := rdb.Del(ctx, "session:"+authorization).Result()
	if err != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"success": s != 0})
}

type DeleteSessions struct {
	Password string `json:"password" validate:"required,min=8,max=32"`
}

func deleteSessionEntries(parsed structs.Session, c *fiber.Ctx) ([]string, error) {
	keys, _, err := rdb.Scan(ctx, 0, "session:"+"*", 0).Result()
	if err != nil {
		return nil, c.Status(500).JSON(errors.ServerRedisError)
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
		return c.Status(401).JSON(errors.AuthorizationMissing)
	}
	var body DeleteSessions
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err := handleValidateErrors(errs, c); err != nil {
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
	var user structs.User
	if err := db.Model(&structs.User{}).Where(&structs.User{ID: parsed.UserID}).Select("password").First(&user).Error; err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	keys, err := deleteSessionEntries(parsed, c)
	if err != nil {
		return err
	}
	return c.Status(200).JSON(fiber.Map{"deleted": len(keys)})
}
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
	cached, err := rdb.Get(ctx, "user:"+parsed.UserID).Result()
	var user structs.ApiUser
	if err == redis.Nil {
		err = db.Model(&structs.User{}).Where(&structs.User{ID: parsed.UserID}).First(&user).Error
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(errors.NotFound)
		} else if err != nil {
			return c.Status(500).JSON(errors.ServerSqlError)
		}
		stringified, err := sonic.Marshal(&user)
		if err != nil {
			return c.Status(500).JSON(errors.ServerStringifyError)
		}
		rdb.Set(ctx, "user:"+parsed.UserID, stringified, 2*time.Minute)
	} else if err != nil {
		return c.Status(500).JSON(errors.ServerRedisError)
	} else {
		err = sonic.UnmarshalString(cached, &user)
		if err != nil {
			fmt.Println(err.Error())
			return c.Status(500).JSON(errors.ServerParseError)
		}
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
	if err := handleValidateErrors(errs, c); err != nil {
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
	if err := handleValidateErrors(errs, c); err != nil {
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
	if err := handleValidateErrors(errs, c); err != nil {
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
	if err := handleValidateErrors(errs, c); err != nil {
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
