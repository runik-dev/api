package routes

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"api/email"
	"api/structs"

	"code.gitea.io/sdk/gitea"
	"github.com/bwmarrin/snowflake"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

var (
	db        *gorm.DB
	rdb       *redis.Client
	env       *structs.Environment
	sender    *email.EmailSender
	generator *snowflake.Node
	_validate *XValidator
	git       *gitea.Client

	ctx      = context.Background()
	validate = validator.New()
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

func DefineRoutes(r *fiber.App, database *gorm.DB, redisDatabase *redis.Client, environment *structs.Environment, emailSender *email.EmailSender, gitClient *gitea.Client) {
	db = database
	rdb = redisDatabase
	env = environment
	sender = emailSender
	git = gitClient
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
	users.Put("/sessions/:totp", confirm2faSignIn)

	users.Get("/me", getMe)
	users.Put("/me/email", putEmail)
	users.Put("/me/password", putPassword)
	users.Delete("/me", deleteMe)
	users.Put("/me/avatar", putAvatar)
	users.Delete("/me/avatar", deleteAvatar)

	users.Post("/verify", postVerify)
	users.Put("/verify/:token", putVerify)

	users.Post("/reset", postReset)
	users.Put("/reset/:token", putReset)

	users.Post("/totp", setUp2FA)
	users.Put("/totp/:code", verify2fa)
	users.Delete("/totp", remove2fa)

	projects := v1.Group("/projects")

	projects.Get("/", getProjects)
	projects.Get("/deployments", getDeployProjects)
	projects.Post("/", createProject)
	projects.Get("/:id/file", getFile)
	projects.Patch("/files", updateContents)
	projects.Get("/:id/files", getContents)
	projects.Get("/:id", getProject)
	projects.Delete("/:id", deleteProject)
}

func emailAvailable(email string) (bool, structs.User) {
	var user structs.User
	found := db.Where("email = ?", email).First(&user).Error
	return found == gorm.ErrRecordNotFound, user
}

func handleValidateErrors(errs []ErrorResponse, c *fiber.Ctx) (error, bool) {
	if len(errs) > 0 {
		errMsgs := make([]string, 0)
		for _, err := range errs {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"[%s]: '%v' | Needs to implement '%s'",
				err.FailedField,
				err.Value,
				err.Tag,
			))
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": errMsgs, "code": "malformed_body"}), true
	} else {
		return nil, false
	}
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
