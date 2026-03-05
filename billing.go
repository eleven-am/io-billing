package billing

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Client struct {
	redis *redis.Client
	store *postgresStore
}

func New(redisClient *redis.Client, db *gorm.DB) *Client {
	return &Client{
		redis: redisClient,
		store: newPostgresStore(db),
	}
}

func (c *Client) Migrate() error {
	return c.store.Migrate()
}
