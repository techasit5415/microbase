package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(ctx context.Context, redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return &RedisStore{client: client}, nil
}

func (r *RedisStore) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	return val, err
}

func (r *RedisStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisStore) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisStore) Flush(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

func (r *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	count, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 && ttl > 0 {
		_ = r.client.Expire(ctx, key, ttl).Err()
	}
	return count, nil
}

func (r *RedisStore) Backend() string { return "redis" }
func (r *RedisStore) Close() error    { return r.client.Close() }
