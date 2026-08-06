package memory

import (
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/jonneyless/gbox/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Memory[T any] struct {
	data       T
	hasData    bool // 标记是否有有效数据
	expireTime time.Time
	mu         sync.RWMutex
	duration   time.Duration
	fetchFunc  func() (T, error)
	logger     *zap.SugaredLogger
}

func NewMemory[T any](duration time.Duration, fetchFunc func() (T, error)) *Memory[T] {
	return &Memory[T]{
		duration:  duration,
		fetchFunc: fetchFunc,
		hasData:   false,
		logger:    logger.GetLogger(),
	}
}

func (c *Memory[T]) Get() T {
	c.mu.RLock()
	if c.hasData && !c.isExpired() {
		result := c.data
		c.mu.RUnlock()
		return result
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重检查
	if c.hasData && !c.isExpired() {
		return c.data
	}

	// 获取新数据
	data, err := c.fetchFunc()
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			if c.hasData {
				c.expireTime = time.Now().Add(c.duration / 2)
				if c.logger != nil {
					c.logger.Errorf("refresh cache failed, using stale data: %v", err)
				}
			} else {
				var zero T
				c.data = zero
				c.hasData = false
				c.expireTime = time.Time{}
				if c.logger != nil {
					c.logger.Errorf("initial cache load failed: %w", err)
				}
			}
			return c.data
		}
	}

	// 验证数据有效性
	if c.isValidData(data) {
		c.data = data
		c.hasData = true
		c.expireTime = time.Now().Add(c.duration)
	} else {
		// 数据无效
		if c.hasData {
			c.expireTime = time.Now().Add(c.duration / 2)
		} else {
			var zero T
			c.data = zero
			c.hasData = false
		}
	}

	return c.data
}

func (c *Memory[T]) ForceRefresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := c.fetchFunc()
	if err != nil {
		return err
	}

	if c.isValidData(data) {
		c.data = data
		c.hasData = true
		c.expireTime = time.Now().Add(c.duration)
	}
	return nil
}

func (c *Memory[T]) isExpired() bool {
	return time.Now().After(c.expireTime)
}

func (c *Memory[T]) isValidData(data T) bool {
	if c.isEmpty(data) {
		return false
	}
	return true
}

// isEmpty 检查数据是否为空
func (c *Memory[T]) isEmpty(data T) bool {
	v := reflect.ValueOf(data)

	// 如果是 nil 指针
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return true
	}

	// 如果是 nil 接口
	if v.Kind() == reflect.Interface && v.IsNil() {
		return true
	}

	// 检查各种集合类型
	switch v.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return v.Len() == 0
	case reflect.String:
		return v.Len() == 0
	case reflect.Struct:
		return false
	default:
		zero := reflect.Zero(v.Type()).Interface()
		return reflect.DeepEqual(data, zero)
	}
}

func (c *Memory[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero T
	c.data = zero
	c.hasData = false
	c.expireTime = time.Time{}
}
