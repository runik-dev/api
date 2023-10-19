package structs

import (
	"time"
)

type Environment struct {
	ConnectionString string
	GlobalAuth       string
	SmtpHost         string
	SmtpPort         string
	SenderEmail      string
	SenderPassword   string
	SenderUsername   string
	RedisAddress     string
	RedisPassword    string
	Port             string
	Rps              string
}
type User struct {
	ID        string `gorm:"uniqueIndex"`
	Email     string `gorm:"uniqueIndex"`
	Password  string `gorm:"notNull"`
	Verified  bool   `gorm:"default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
type ApiUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}
type Session struct {
	UserID string
	IP     string
}
