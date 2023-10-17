package errors

import "github.com/gofiber/fiber/v2"

var AuthorizationInvalid = fiber.Map{"code": "authorization_invalid"}
var AuthorizationMissing = fiber.Map{"code": "authorization_missing"}

func MalformedBody(err error) fiber.Map {
	return fiber.Map{"code": "malformed_body", "error": err.Error()}
}

var UserAlreadyVerified = fiber.Map{"code": "user_already_verified"}
var UserEmailTaken = fiber.Map{"code": "user_email_taken"}
var UserCredentialsInvalid = fiber.Map{"code": "user_credentials_invalid"}
var MissingParameter = fiber.Map{"code": "missing_parameter"}
var NotFound = fiber.Map{"code": "not_found"}

var ServerEmailSend = fiber.Map{"code": "server_failed_email"}
var ServerHash = fiber.Map{"code": "server_failed_hash"}
var ServerParseError = fiber.Map{"code": "server_failed_parse"}
var ServerStringifyError = fiber.Map{"code": "server_failed_stringify"}
var ServerTokenGenerate = fiber.Map{"code": "server_failed_token"}
var ServerSqlError = fiber.Map{"code": "server_sql_error"}
var ServerRedisError = fiber.Map{"code": "server_redis_error"}
