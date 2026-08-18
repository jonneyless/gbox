package gbox

import (
	"fmt"
	"sync"
	"time"

	"github.com/jonneyless/gbox/cache"
)

type IdempotentLimiter struct {
	cache sync.Map
	ttl   time.Duration
}

func NewIdempotentLimiter(ttl time.Duration) *IdempotentLimiter {
	return &IdempotentLimiter{
		ttl: ttl,
	}
}

func (l *IdempotentLimiter) Process(id string) bool {
	now := time.Now()

	actual, loaded := l.cache.LoadOrStore(id, now)
	if loaded {
		lastTime := actual.(time.Time)
		if now.Sub(lastTime) < l.ttl {
			return false
		}

		l.cache.Store(id, now)
		return true
	}

	return true
}

func (l *IdempotentLimiter) CleanExpired() {
	now := time.Now()
	l.cache.Range(func(key, value any) bool {
		if now.Sub(value.(time.Time)) >= l.ttl {
			l.cache.Delete(key)
		}
		return true
	})
}

type RedisLimiter struct {
	client *cache.Redis
	ttl    time.Duration
}

func NewRedisLimiter(client *cache.Redis, ttl time.Duration) *RedisLimiter {
	return &RedisLimiter{
		client: client,
		ttl:    ttl,
	}
}

func (l *RedisLimiter) Process(id string) bool {
	key := fmt.Sprintf("limiter:%s", id)

	return l.client.SetNX(key, time.Now().Unix(), l.ttl)
}
