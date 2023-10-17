package database

import (
	"log"
	"runik-api/structs"

	"github.com/go-redis/redis/v8"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect(env *structs.Environment) *gorm.DB {
	db, err := gorm.Open(postgres.Open(env.ConnectionString), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect to db", err)
	}
	db.AutoMigrate(&structs.User{})

	return db
}
func RedisConnect(env *structs.Environment) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     env.RedisAddress,
		Password: env.RedisPassword,
		DB:       0,
	})
}
