package structs

import (
	"time"
)

type Environment struct {
	TursoUrl   string
	TursoToken string

	ApiAuthentication string

	SmtpHost     string
	SmtpPort     string
	SmtpUsername string

	SenderEmail    string
	SenderPassword string

	RedisAddress  string
	RedisPassword string

	Port              string
	RequestsPerSecond string

	GitToken    string
	GitUrl      string
	GitUsername string

	MinioEndpoint     string
	MinioAccessKeyId  string
	MinioAccessKey    string
	MinioAvatarBucket string
}
type User struct {
	ID       string `gorm:"type:bigint;primaryKey"`
	Email    string `gorm:"uniqueIndex"`
	Password string `gorm:"notNull"`
	Verified bool   `gorm:"default:false"`

	TotpSecret   string
	TotpVerified bool `gorm:"default:false"`

	CreatedAt time.Time
	UpdatedAt time.Time
}
type ApiUser struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Verified     bool   `json:"verified"`
	TotpVerified bool   `json:"totp_verified"`
}
type Project struct {
	ID        string `gorm:"uniqueIndex"`
	UserID    string `gorm:"type:bigint"`
	User      User   `gorm:"foreignKey:UserID"`
	Name      string `gorm:"notNull"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
type ApiProject struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Session struct {
	UserID string
	IP     string
}
