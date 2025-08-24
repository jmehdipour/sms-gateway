package db

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisOpts struct {
	Addr        string        // "127.0.0.1:6379"
	Password    string        // optional
	DB          int           // default 0
	DialTimeout time.Duration // default 5s
}

func NewRedisClient(opts RedisOpts) (*redis.Client, error) {
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 5 * time.Second
	}
	
	rdb := redis.NewClient(&redis.Options{
		Addr:        opts.Addr,
		Password:    opts.Password,
		DB:          opts.DB,
		DialTimeout: opts.DialTimeout,
	})
	ctx, cancel := context.WithTimeout(context.Background(), opts.DialTimeout)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}

	return rdb, nil
}
