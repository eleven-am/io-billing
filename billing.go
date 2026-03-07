package billing

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Client struct {
	redis *redis.Client
	store *postgresStore
	opts  Options
}

func New(redisClient *redis.Client, db *gorm.DB) *Client {
	return NewWithOptions(redisClient, db, nil)
}

func NewWithOptions(redisClient *redis.Client, db *gorm.DB, opts *Options) *Client {
	return &Client{
		redis: redisClient,
		store: newPostgresStore(db),
		opts:  withDefaults(opts),
	}
}

func (c *Client) Migrate() error {
	return c.store.Migrate()
}
