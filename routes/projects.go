package routes

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/bytedance/sonic"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
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
func getProject(c *fiber.Ctx) error {
	projectId := c.Params("id")
	if projectId == "" {
		return c.Status(http.StatusBadRequest).JSON(errors.MissingParameter)
	}
	var project structs.ApiProject

	err := db.Model(&structs.Project{}).Where(&structs.Project{ID: projectId}).First(&project).Error
	if err == gorm.ErrRecordNotFound {
		return c.Status(http.StatusNotFound).JSON(errors.NotFound)
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerSqlError)
	}
	return c.Status(http.StatusOK).JSON(project)
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

type DeleteProject struct {
	Password string `json:"password" validate:"required,min=8,max=32"`
}

func deleteProject(c *fiber.Ctx) error {
	projectId := c.Params("id")
	if projectId == "" {
		return c.Status(http.StatusBadRequest).JSON(errors.MissingParameter)
	}
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

	var body DeleteProject
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(errors.MalformedBody(err))
	}
	errs := _validate.Validate(body)
	if err, rtrn := handleValidateErrors(errs, c); rtrn {
		return err
	}

	var user structs.User
	// Get user and error if not found
	if err := db.Model(&structs.User{}).Where(&structs.User{ID: parsed.UserID}).Select("password").First(&user).Error; err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}
	// Check password and error if it is wrong
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		return c.Status(400).JSON(errors.UserCredentialsInvalid)
	}

	var project structs.Project
	// Get project and error if not found
	err = db.Where(&structs.Project{ID: projectId}).First(&project).Error
	if err == gorm.ErrRecordNotFound {
		return c.Status(http.StatusNotFound).JSON(errors.NotFound)
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerSqlError)
	}

	err = db.Delete(&project).Error
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerSqlError)
	}

	// Delete repo and recreate project if it fails
	name := parsed.UserID + "-" + projectId
	_, err = git.DeleteRepo("runikbot", name)
	if err != nil {
		db.Create(project)
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerGitError)
	}
	return c.Status(http.StatusNoContent).Send(nil)
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
	_, err = http.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerGitError)
	}

	return c.Status(http.StatusOK).JSON(result)
}
func getContents(c *fiber.Ctx) error {
	projectId := c.Params("id")
	if projectId == "" {
		return c.Status(http.StatusBadRequest).JSON(errors.MissingParameter)
	}
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
	name := parsed.UserID + "-" + projectId
	branch, _, err := git.GetRepoBranch("runikbot", name, "dev")

	if err != nil && err.Error() == "The target couldn't be found." {
		return c.Status(http.StatusNotFound).JSON(errors.NotFound)
	}
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(errors.ServerGitError)
	}
	sha := branch.Commit.ID
	url := env.GitUrl + "/api/v1/repos/" + env.GitUsername + "/" + name + "/git/trees/" + sha + "?recursive=true&token=" + env.GitToken
	req, _ := http.Get(url)
	data, err := convertToFiles(req.Body)
	if err != nil {
		fmt.Println("err", err.Error())
	}
	return c.Status(req.StatusCode).JSON(data)
}

func readToString(reader io.ReadCloser) (string, error) {
    defer reader.Close()

    data, err := io.ReadAll(reader)
    if err != nil {
        return "", err
    }

    return string(data), nil
}
func convertToFiles(reader io.ReadCloser) (interface{}, error) {
	jsonString, err := readToString(reader)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("node", "./routes/convert.js", "--json", jsonString)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error executing command:", err)
		return nil, err
	}
	var json []interface{}
	err = sonic.Unmarshal(output, &json)
	if err != nil {
		fmt.Println("Error executing command:", err)
		return nil, err
	}
	return json, nil
}
func getFile(c *fiber.Ctx) error {
	path := c.Query("path")
	if path == "" {
		return c.Status(http.StatusBadRequest).JSON(errors.MissingParameter)
	}
	projectId := c.Params("id")
	if projectId == "" {
		return c.Status(http.StatusBadRequest).JSON(errors.MissingParameter)
	}
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
	name := parsed.UserID + "-" + projectId
	file, _, _ := git.GetFile(env.GitUsername, name, "dev", path)
	if file == nil {
		return c.Status(http.StatusNotFound).JSON(errors.NotFound)
	}
	return c.Status(http.StatusOK).Send(file)
}
