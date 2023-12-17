package routes

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/bytedance/sonic"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"runik-api/errors"
	"runik-api/structs"
)

func getProjects(c *fiber.Ctx) error {
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

	var projects []structs.ApiProject
	val, err := rdb.Get(ctx, "projects:"+parsed.UserID).Result()

	if err != nil {
		db.Model(&structs.Project{}).Where(&structs.Project{UserID: parsed.UserID}).Find(&projects)

		json, err := sonic.Marshal(&projects)
		if err == nil {
			rdb.Set(ctx, "projects:"+parsed.UserID, json, 2*time.Minute)
		}
	} else {
		err := sonic.UnmarshalString(val, &projects)
		if err != nil {
			return c.Status(500).JSON(errors.ServerParseError)
		}
	}

	return c.Status(http.StatusOK).JSON(projects)
}

type CreateBody struct {
	Name string `json:"name" validate:"required,min=4,max=64"`
}

func createProject(c *fiber.Ctx) error {
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

	var body CreateBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {
		return err
	}
	id := generator.Generate()
	name := parsed.UserID + "-" + id.String()
	_, _, err = git.CreateRepoFromTemplate(env.GitUsername, env.GitTemplate, gitea.CreateRepoFromTemplateOption{Owner: env.GitUsername, Name: name, Private: true, GitContent: true, Description: body.Name})
	if err != nil {
		fmt.Println(err.Error())
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerGitError)
	}
	_, _, err = git.CreateBranch(env.GitUsername, name, gitea.CreateBranchOption{BranchName: "dev", OldBranchName: "prod"})
	if err != nil {
		fmt.Println(err.Error())
		git.DeleteRepo(env.GitUsername, name)
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerGitError)
	}
	project := structs.Project{ID: id.String(), UserID: parsed.UserID, Name: body.Name}
	err = db.Create(&project).Error
	if err != nil {
		fmt.Println(err.Error())
		git.DeleteRepo(env.GitUsername, name)
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerSqlError)
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"id": id.String()})
}

type UpdateBody struct {
	ProjectId string            `json:"project_id" validate:"required"`
	Files     map[string]string `json:"files" validate:"required"`
	Delete    []string          `json:"delete" validate:"required"`
}
type fileInfo struct {
	Path      string  `json:"path"`
	Content   *string `json:"content,omitempty"`
	Operation string  `json:"operation"`
	SHA       *string `json:"sha,omitempty"`
}

func updateContents(c *fiber.Ctx) error {
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

	var body UpdateBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {
		return err
	}
	var project structs.Project
	err = db.Where(&structs.Project{ID: body.ProjectId}).First(&project).Error
	if err == gorm.ErrRecordNotFound {
		return c.Status(http.StatusNotFound).JSON(errors.NotFound)
	} else if err != nil {
		return c.Status(500).JSON(errors.ServerSqlError)
	}
	if project.UserID != parsed.UserID {
		return c.Status(http.StatusForbidden).JSON(errors.ProjectNoAccess)
	}

	name := parsed.UserID + "-" + body.ProjectId

	var wg sync.WaitGroup
	var mu sync.Mutex
	var result []fileInfo
	for path, fileContent := range body.Files {
		wg.Add(1)
		go func(p, c string) {
			defer wg.Done()
			content, _, err := git.GetContents("runikbot", name, "dev", p)

			encoded := base64.StdEncoding.EncodeToString([]byte(c))
			if err != nil {
				mu.Lock()
				defer mu.Unlock()
				result = append(result, fileInfo{
					Path:      p,
					Content:   &encoded,
					Operation: "create",
				})
			} else {
				if *content.Content == encoded {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				result = append(result, fileInfo{
					Path:      p,
					Content:   &encoded,
					Operation: "update",
					SHA:       &content.SHA,
				})
			}

		}(path, fileContent)
	}
	for _, path := range body.Delete {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			content, _, err := git.GetContents("runikbot", name, "dev", p)
			if err != nil {
				return
			}

			mu.Lock()
			defer mu.Unlock()
			result = append(result, fileInfo{
				Path:      p,
				Operation: "delete",
				SHA:       &content.SHA,
			})
		}(path)
	}
	wg.Wait()
	if len(result) == 0 {
		return c.Status(http.StatusNoContent).Send(nil)
	}
	reqBody, err := sonic.Marshal(fiber.Map{"author": fiber.Map{"name": parsed.UserID}, "branch": "dev", "message": "update contents", "files": result})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerStringifyError)
	}
	url := env.GitUrl + "/api/v1/repos/" + env.GitUsername + "/" + name + "/contents?ref=dev&token=" + env.GitToken
	req, err := http.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerGitError)
	}
	fmt.Println(req.Status)
	return c.Status(http.StatusOK).JSON(result)
}
