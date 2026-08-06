package cache

import (
	"context"
	"errors"
	"fmt"
	"gbox/logger"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var rdb *Redis

func GetRedis() *Redis {
	return rdb
}

type RedisParams struct {
	Host     string
	Port     int
	Password string
	DB       int
	Prefix   string
}

func InitRedis(ctx context.Context, c *RedisParams) {
	rdb = &Redis{
		ctx:    ctx,
		logger: logger.GetLogger(),
		prefix: c.Prefix,
		client: redis.NewClient(&redis.Options{
			Addr:         fmt.Sprintf("%s:%d", c.Host, c.Port),
			Password:     c.Password,
			DB:           c.DB,
			PoolSize:     100,
			MinIdleConns: 5,
		}),
	}
}

type Redis struct {
	ctx    context.Context
	prefix string
	logger *zap.SugaredLogger
	client *redis.Client
}

type RedisPipeline struct {
	ctx  context.Context
	pipe redis.Pipeliner
}

func (r *Redis) SetContext(ctx context.Context) {
	r.ctx = ctx
}

func (r *Redis) Log() *zap.SugaredLogger {
	return r.logger
}

func (r *Redis) Ping() {
	_, err := r.client.Ping(r.ctx).Result()
	if err != nil {
		r.logger.DPanicf("连接Redis失败: %v", err)
	}
}

func (r *Redis) Close() {
	r.logger.Debug("Redis 连接已关闭")
	err := r.client.Close()
	if err != nil {
		panic(err)
	}
}

func (r *Redis) Mutex(key string) *redsync.Mutex {
	pool := goredis.NewPool(r.client)
	rs := redsync.New(pool)
	return rs.NewMutex(key,
		redsync.WithExpiry(10*time.Second),
		redsync.WithTries(3),
		redsync.WithRetryDelay(500*time.Millisecond),
	)
}

func (r *Redis) GetKey(key string) string {
	return fmt.Sprintf("%s:%s", r.prefix, key)
}

func (r *Redis) Set(key string, value any, expiration time.Duration) error {
	return r.client.Set(r.ctx, key, value, expiration).Err()
}

func (r *Redis) SetNX(key string, value any, expiration time.Duration) bool {
	return r.client.SetNX(r.ctx, key, value, expiration).Val()
}

func (r *Redis) Get(key string) (string, error) {
	return r.client.Get(r.ctx, key).Result()
}

func (r *Redis) Del(key string) error {
	return r.client.Del(r.ctx, key).Err()
}

func (r *Redis) RPush(key string, value any) error {
	return r.client.RPush(r.ctx, r.GetKey(key), value).Err()
}

func (r *Redis) LPop(key string) (string, error) {
	return r.client.LPop(r.ctx, r.GetKey(key)).Result()
}

func (r *Redis) BLPop(key string, timeout time.Duration) ([]string, error) {
	return r.client.BLPop(r.ctx, timeout, r.GetKey(key)).Result()
}

func (r *Redis) HGetAll(key string) (map[string]string, error) {
	return r.client.HGetAll(r.ctx, r.GetKey(key)).Result()
}

func (r *Redis) HGet(key string, field string) (string, error) {
	return r.client.HGet(r.ctx, r.GetKey(key), field).Result()
}

func (r *Redis) HSet(key string, values ...any) error {
	return r.client.HSet(r.ctx, r.GetKey(key), values...).Err()
}

func (r *Redis) HDel(key string, fields ...string) (int64, error) {
	return r.client.HDel(r.ctx, r.GetKey(key), fields...).Result()
}

func (r *Redis) Exists(key string) (int64, error) {
	return r.client.Exists(r.ctx, r.GetKey(key)).Result()
}

func (r *Redis) Incr(key string) (int64, error) {
	return r.client.Incr(r.ctx, r.GetKey(key)).Result()
}

func (r *Redis) Expire(key string, expiration time.Duration) (bool, error) {
	return r.client.Expire(r.ctx, r.GetKey(key), expiration).Result()
}

func (r *Redis) DoOnceWithinPeriod(key string, period time.Duration, fn func() error) (executed bool, err error) {
	lockKey := r.GetKey(fmt.Sprintf("once:%s", key))

	success, err := r.client.SetNX(r.ctx, lockKey, time.Now().Unix(), period).Result()
	if err != nil {
		return false, fmt.Errorf("获取分布式锁失败: %v", err)
	}

	if !success {
		return false, nil
	}

	defer func() {
		if p := recover(); p != nil {
			_ = r.client.Del(r.ctx, lockKey).Err()
			panic(p)
		}
	}()

	if err := fn(); err != nil {
		_ = r.client.Del(r.ctx, lockKey).Err()
		return true, err
	}

	return true, nil
}

func (r *Redis) DoOnceWithinPeriodWithRetry(key string, period time.Duration, maxRetries int, fn func() error) (executed bool, err error) {
	lockKey := r.GetKey(fmt.Sprintf("once:%s", key))

	success, err := r.client.SetNX(r.ctx, lockKey, time.Now().Unix(), period).Result()
	if err != nil {
		return false, fmt.Errorf("获取分布式锁失败: %v", err)
	}

	if !success {
		return false, nil
	}

	var execErr error
	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			backoff := min(time.Duration(1<<uint(i-1))*100*time.Millisecond, 5*time.Second)
			time.Sleep(backoff)
			r.logger.Debugf("重试业务逻辑，第 %d 次重试", i)
		}

		execErr = fn()
		if execErr == nil {
			return true, nil
		}
	}

	_ = r.client.Del(r.ctx, lockKey).Err()
	return true, execErr
}

func (r *Redis) GetRemainingLockTime(key string) (time.Duration, error) {
	lockKey := r.GetKey(fmt.Sprintf("once:%s", key))
	ttl, err := r.client.TTL(r.ctx, lockKey).Result()
	if err != nil {
		return 0, err
	}
	if ttl < 0 {
		return 0, errors.New("锁不存在或已过期")
	}
	return ttl, nil
}

func (r *Redis) ForceReleaseLock(key string) error {
	lockKey := r.GetKey(fmt.Sprintf("once:%s", key))
	return r.client.Del(r.ctx, lockKey).Err()
}

func (r *Redis) Pipeline() RedisPipeline {
	return RedisPipeline{
		ctx:  r.ctx,
		pipe: r.client.Pipeline(),
	}
}

func (p *RedisPipeline) Del(key string) *redis.IntCmd {
	return p.pipe.Del(p.ctx, key)
}

func (p *RedisPipeline) Exec() ([]redis.Cmder, error) {
	return p.pipe.Exec(p.ctx)
}
