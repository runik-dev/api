package database

import (
	"log"

	"api/structs"

	"github.com/go-redis/redis/v8"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect(env *structs.Environment) *gorm.DB {
	db, err := gorm.Open(postgres.Open(env.PostgresConnection), &gorm.Config{})
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
