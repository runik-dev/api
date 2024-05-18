package database

import (
	"log"

	"api/structs"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
  	"github.com/ekristen/gorm-libsql"
)

func Connect(env *structs.Environment) *gorm.DB {
	url := env.TursoUrl + "?authToken=" + env.TursoToken
	db, err := gorm.Open(libsql.Open(url), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect to db", err)
	}
	db.AutoMigrate(&structs.User{}, &structs.Project{})

	return db
}
func RedisConnect(env *structs.Environment) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     env.RedisAddress,
		Password: env.RedisPassword,
		DB:       0,
	})
}
