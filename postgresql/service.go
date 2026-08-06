package postgresql

import (
	"errors"
	"fmt"
	"gbox/cache"
	"gbox/logger"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/spf13/cast"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DefaultCacheTTL = time.Hour
	MaxCacheTTL     = 24 * time.Hour
)

var (
	ErrEmptyUpdates    = errors.New("updates cannot be empty")
	ErrEmptyConditions = errors.New("conditions cannot be empty")
	ErrEmptyModels     = errors.New("models cannot be empty")
	ErrEmptyIDs        = errors.New("ids cannot be empty")
	ErrNotFound        = errors.New("not found")
)

type BaseService[T any] struct {
	Prefix    string
	TableName string
	cache     *cache.Redis
}

type CacheConfig struct {
	TTL       time.Duration
	EnableNil bool
}

type Condition struct {
	Field    string
	Operator string
	Value    any
}

type QueryOptions struct {
	Preload    string
	PreloadOpt []any
	Joins      string
	Orders     []QueryOrder
	Select     []string
	Page       int
	PageSize   int
}

type QueryOption func(*QueryOptions)

type QueryOrder struct {
	OrderBy    string
	Descending bool
}

func WithOrder(orderBy string, descending bool) QueryOption {
	return func(o *QueryOptions) {
		o.Orders = append(o.Orders, QueryOrder{
			OrderBy:    orderBy,
			Descending: descending,
		})
	}
}

func WithPreload(table string, opt ...any) QueryOption {
	return func(o *QueryOptions) {
		o.Preload = table
		o.PreloadOpt = append(o.PreloadOpt, opt...)
	}
}

func WithJoins(joins string) QueryOption {
	return func(o *QueryOptions) {
		o.Joins = joins
	}
}

func WithSelect(fields ...string) QueryOption {
	return func(o *QueryOptions) {
		o.Select = fields
	}
}

func WithPagination(page, pageSize int) QueryOption {
	return func(o *QueryOptions) {
		o.Page = page
		o.PageSize = pageSize
	}
}

func (srv *BaseService[T]) Rds() *cache.Redis {
	if srv.cache == nil {
		srv.cache = cache.GetRedis()
	}

	return srv.cache
}

func (srv *BaseService[T]) WithTransaction(fn func(tx *gorm.DB) error) error {
	return DB().Transaction(fn)
}

func (srv *BaseService[T]) GetById(id int64) (*T, error) {
	var model *T

	err := DB().Where("id = ?", id).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w", ErrNotFound)
		}
		return nil, err
	}

	return model, nil
}

func (srv *BaseService[T]) GetWithCache(id int64) (*T, error) {
	return srv.GetWithCacheConfig(id, CacheConfig{TTL: DefaultCacheTTL, EnableNil: false})
}

func (srv *BaseService[T]) GetWithCacheConfig(id int64, config CacheConfig) (*T, error) {
	if srv.Prefix == "" {
		return srv.GetById(id)
	}

	if config.TTL > MaxCacheTTL {
		config.TTL = MaxCacheTTL
	}

	cacheKey := fmt.Sprintf("%s:%d", srv.Prefix, id)

	data, err := srv.Rds().Get(cacheKey)
	if err == nil && data != "" {
		if data == "NULL" && config.EnableNil {
			return nil, fmt.Errorf("%w", ErrNotFound)
		}

		item := new(T)
		if err = sonic.Unmarshal([]byte(data), item); err == nil {
			return item, nil
		}
		_ = srv.Rds().Del(cacheKey)
	}

	item, err := srv.GetById(id)
	if err != nil {
		return nil, err
	}

	if item == nil {
		if config.EnableNil {
			_ = srv.Rds().Set(cacheKey, "NULL", config.TTL)
		}
		return nil, fmt.Errorf("%w", ErrNotFound)
	}

	cacheData, _ := sonic.Marshal(item)
	_ = srv.Rds().Set(cacheKey, string(cacheData), config.TTL)
	return item, nil
}

func (srv *BaseService[T]) GetByField(field string, value any, opts ...QueryOption) (*T, error) {
	return srv.GetByCondition([]Condition{{Field: field, Value: value}}, opts...)
}

func (srv *BaseService[T]) GetByCondition(conditions []Condition, opts ...QueryOption) (*T, error) {
	if len(conditions) == 0 {
		return nil, ErrEmptyConditions
	}

	options := &QueryOptions{
		Page:     0,
		PageSize: 0,
		Orders:   []QueryOrder{},
		Select:   []string{},
		Preload:  "",
		Joins:    "",
	}
	for _, opt := range opts {
		opt(options)
	}

	var model *T

	query := srv.buildQuery(conditions)

	if options.Preload != "" {
		query = query.Preload(options.Preload, options.PreloadOpt...)
	}

	if len(options.Select) > 0 {
		query = query.Select(options.Select)
	}

	if options.Joins != "" {
		query = query.Joins(options.Joins)
	}

	if len(options.Orders) > 0 {
		for _, order := range options.Orders {
			if order.Descending {
				query = query.Order(order.OrderBy + " DESC")
			} else {
				query = query.Order(order.OrderBy + " ASC")
			}
		}
	}

	err := query.First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w", ErrNotFound)
		}
		return nil, err
	}

	return model, nil
}

func (srv *BaseService[T]) GetIdsByCondition(conditions []Condition) ([]int64, error) {
	return srv.PluckInt64("id", conditions)
}

func (srv *BaseService[T]) PluckString(column string, conditions []Condition) ([]string, error) {
	var items []string
	err := srv.buildQuery(conditions).Pluck(column, &items).Error
	return items, err
}

func (srv *BaseService[T]) PluckInt64(column string, conditions []Condition) ([]int64, error) {
	var items []int64
	err := srv.buildQuery(conditions).Pluck(column, &items).Error
	return items, err
}

func (srv *BaseService[T]) Find(conditions []Condition, opts ...QueryOption) ([]*T, error) {
	if len(conditions) == 0 {
		return nil, ErrEmptyConditions
	}

	options := &QueryOptions{
		Page:     0,
		PageSize: 0,
		Orders:   []QueryOrder{},
		Select:   []string{},
		Preload:  "",
		Joins:    "",
	}
	for _, opt := range opts {
		opt(options)
	}

	query := srv.buildQuery(conditions)

	if options.Preload != "" {
		query = query.Preload(options.Preload, options.PreloadOpt...)
	}

	if len(options.Select) > 0 {
		query = query.Select(options.Select)
	}

	if options.Joins != "" {
		query = query.Joins(options.Joins)
	}

	if len(options.Orders) > 0 {
		for _, order := range options.Orders {
			if order.Descending {
				query = query.Order(order.OrderBy + " DESC")
			} else {
				query = query.Order(order.OrderBy + " ASC")
			}
		}
	}

	if options.PageSize > 0 {
		if options.Page > 0 {
			offset := (options.Page - 1) * options.PageSize
			query = query.Offset(offset)
		}

		query = query.Limit(options.PageSize)
	}

	var models []*T
	err := query.Find(&models).Error
	return models, err
}

func (srv *BaseService[T]) FindAll() ([]*T, error) {
	var models []*T
	err := DB().Model(new(T)).Find(&models).Error
	return models, err
}

func (srv *BaseService[T]) Create(model *T) error {
	err := DB().Create(model).Error
	if err != nil {
		return err
	}

	return nil
}

func (srv *BaseService[T]) CreateBatch(models []*T, batchSize int) error {
	if len(models) == 0 {
		return ErrEmptyModels
	}

	if batchSize <= 0 || batchSize > 100 {
		batchSize = 100
	}

	err := DB().CreateInBatches(models, batchSize).Error
	if err != nil {
		return err
	}

	return nil
}

func (srv *BaseService[T]) CreateAndGetId(model *T) (int64, error) {
	err := DB().Create(model).Error
	if err != nil {
		return 0, err
	}

	id := srv.getModelId(model)

	return id, nil
}

func (srv *BaseService[T]) FirstOrCreate(model *T, conditions []Condition) error {
	err := srv.buildQuery(conditions).FirstOrCreate(model).Error
	if err != nil {
		return err
	}

	return nil
}

func (srv *BaseService[T]) UpdateJson(id int64, field string, jsonKey string, value any) error {
	var err error
	if value == nil {
		sql := fmt.Sprintf("UPDATE %s SET %s = %s - $1::text WHERE id = $2", srv.TableName, field, field)
		err = DB().Exec(sql, jsonKey, id).Error
	} else {
		err = DB().Model(new(T)).Where("id = ?", id).UpdateColumn(field, datatypes.JSONSet(field).Set(fmt.Sprintf("{%s}", jsonKey), value)).Error
	}
	if err != nil {
		return err
	}

	srv.CleanCache(id)

	return nil
}

func (srv *BaseService[T]) UpdateField(id int64, field string, value any) error {
	err := DB().Model(new(T)).Where("id = ?", id).UpdateColumn(field, value).Error
	if err != nil {
		return err
	}

	srv.CleanCache(id)

	return nil
}

func (srv *BaseService[T]) Update(id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return ErrEmptyUpdates
	}

	err := DB().Model(new(T)).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		return err
	}

	srv.CleanCache(id)

	return nil
}

func (srv *BaseService[T]) UpdateModel(id int64, model *T) error {
	err := DB().Where("id = ?", id).Save(model).Error
	if err != nil {
		return err
	}

	srv.CleanCache(id)

	return nil
}

func (srv *BaseService[T]) UpdateByCondition(conditions []Condition, updates map[string]any) (int64, error) {
	if len(conditions) == 0 {
		return 0, ErrEmptyConditions
	}

	var ids []int64
	rows := int64(0)
	err := DB().Transaction(func(tx *gorm.DB) error {
		if srv.Prefix != "" {
			if err := srv.buildQueryWithTx(tx, conditions).Clauses(clause.Locking{Strength: "UPDATE"}).Pluck("id", &ids).Error; err != nil {
				return err
			}
		}

		result := srv.buildQueryWithTx(tx, conditions).Updates(updates)
		if result.Error != nil {
			return result.Error
		}

		rows = result.RowsAffected

		return nil
	})

	if err == nil {
		srv.CleanCacheBatch(ids)
	}

	return rows, err
}

func (srv *BaseService[T]) Increase(id int64, field string, value any) error {
	var val any
	switch value.(type) {
	case decimal.Decimal:
		val = value.(decimal.Decimal).String()
	case int:
		val = value.(int)
	case int64:
		val = value.(int64)
	case float64:
		val = value.(float64)
	default:
		val = cast.ToInt(value)
	}
	return srv.Update(id, map[string]any{field: gorm.Expr(fmt.Sprintf("%s + ?", field), val)})
}

func (srv *BaseService[T]) Reduce(id int64, field string, value any) error {
	var val any
	switch value.(type) {
	case decimal.Decimal:
		val = value.(decimal.Decimal).String()
	case int:
		val = value.(int)
	case int64:
		val = value.(int64)
	case float64:
		val = value.(float64)
	default:
		val = cast.ToInt(value)
	}
	return srv.Update(id, map[string]any{field: gorm.Expr(fmt.Sprintf("%s - ?", field), val)})
}

func (srv *BaseService[T]) Delete(id int64) error {
	err := DB().Delete(new(T), id).Error
	if err != nil {
		return err
	}

	srv.CleanCache(id)

	return nil
}

func (srv *BaseService[T]) DeleteBatch(ids []int64) error {
	if len(ids) == 0 {
		return ErrEmptyIDs
	}

	err := DB().Delete(new(T), ids).Error
	if err != nil {
		return err
	}

	srv.CleanCacheBatch(ids)

	return nil
}

func (srv *BaseService[T]) DeleteByCondition(conditions []Condition) (int64, error) {
	if len(conditions) == 0 {
		return 0, ErrEmptyConditions
	}

	var ids []int64
	var err error

	if srv.Prefix != "" {
		ids, err = srv.GetIdsByCondition(conditions)
		if err != nil {
			return 0, err
		}
	}

	result := srv.buildQuery(conditions).Delete(new(T))

	if result.Error != nil {
		return 0, result.Error
	}

	srv.CleanCacheBatch(ids)

	return result.RowsAffected, nil
}

func (srv *BaseService[T]) ForceDelete(id int64) error {
	err := DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(new(T), id).Error; err != nil {
			return err
		}
		return nil
	})

	if err == nil {
		srv.CleanCache(id)
	}

	return err
}

func (srv *BaseService[T]) ForceDeleteBatch(ids []int64) error {
	err := DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(new(T), ids).Error; err != nil {
			return err
		}
		return nil
	})

	if err == nil {
		srv.CleanCacheBatch(ids)
	}

	return err
}

func (srv *BaseService[T]) Restore(id int64) error {
	err := DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Model(new(T)).Where("id = ?", id).Update("deleted_at", nil).Error; err != nil {
			return err
		}
		return nil
	})

	if err == nil {
		srv.CleanCache(id)
	}

	return err
}

func (srv *BaseService[T]) ExistsByCondition(conditions []Condition) (bool, error) {
	count, err := srv.CountByCondition(conditions)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (srv *BaseService[T]) CountByCondition(conditions []Condition) (int64, error) {
	if len(conditions) == 0 {
		return 0, ErrEmptyConditions
	}

	var count int64

	err := srv.buildQuery(conditions).Count(&count).Error
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (srv *BaseService[T]) CleanCache(id int64) {
	if srv.Prefix == "" || id == 0 {
		return
	}

	cacheKey := fmt.Sprintf("%s:%d", srv.Prefix, id)
	_ = srv.Rds().Del(cacheKey)
}

func (srv *BaseService[T]) CleanCacheBatch(ids []int64) {
	if srv.Prefix == "" || len(ids) == 0 {
		return
	}

	pipe := srv.Rds().Pipeline()
	for _, id := range ids {
		if id == 0 {
			continue
		}

		cacheKey := fmt.Sprintf("%s:%d", srv.Prefix, id)
		pipe.Del(cacheKey)
	}
	_, _ = pipe.Exec()
}

func (srv *BaseService[T]) buildQueryCore(db *gorm.DB, conditions []Condition) *gorm.DB {
	query := db.Model(new(T))

	for _, cond := range conditions {
		if !isValidFieldName(cond.Field) {
			continue
		}

		operator := strings.ToUpper(cond.Operator)
		switch operator {
		case "=", "==":
			query = query.Where(fmt.Sprintf("%s = ?", cond.Field), cond.Value)
		case ">":
			query = query.Where(fmt.Sprintf("%s > ?", cond.Field), cond.Value)
		case ">=":
			query = query.Where(fmt.Sprintf("%s >= ?", cond.Field), cond.Value)
		case "<":
			query = query.Where(fmt.Sprintf("%s < ?", cond.Field), cond.Value)
		case "<=":
			query = query.Where(fmt.Sprintf("%s <= ?", cond.Field), cond.Value)
		case "!=", "<>":
			query = query.Where(fmt.Sprintf("%s != ?", cond.Field), cond.Value)
		case "LIKE":
			query = query.Where(fmt.Sprintf("%s LIKE ?", cond.Field), cond.Value)
		case "IN":
			query = query.Where(fmt.Sprintf("%s IN (?)", cond.Field), cond.Value)
		case "NOT IN":
			query = query.Where(fmt.Sprintf("%s NOT IN (?)", cond.Field), cond.Value)
		case "BETWEEN":
			if values, ok := cond.Value.([]any); ok && len(values) == 2 {
				query = query.Where(fmt.Sprintf("%s BETWEEN ? AND ?", cond.Field), values[0], values[1])
			}
		case "IS NULL":
			query = query.Where(fmt.Sprintf("%s IS NULL", cond.Field))
		case "IS NOT NULL":
			query = query.Where(fmt.Sprintf("%s IS NOT NULL", cond.Field))
		default:
			query = query.Where(fmt.Sprintf("%s = ?", cond.Field), cond.Value)
		}
	}

	return query
}

func (srv *BaseService[T]) buildQuery(conditions []Condition) *gorm.DB {
	return srv.buildQueryCore(DB(), conditions)
}

func (srv *BaseService[T]) buildQueryWithTx(tx *gorm.DB, conditions []Condition) *gorm.DB {
	return srv.buildQueryCore(tx, conditions)
}

func (srv *BaseService[T]) getModelId(model *T) int64 {
	return getIdValue(model, "ID")
}

func isValidFieldName(fieldName string) bool {
	if fieldName == "" {
		return false
	}
	for _, ch := range fieldName {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '.' || ch == '`') {
			return false
		}
	}
	return true
}

func getIdValue(model any, fieldName string) int64 {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		structField := t.Field(i)
		if strings.EqualFold(structField.Name, fieldName) {
			fieldVal := v.Field(i)
			if fieldVal.Kind() == reflect.Int64 {
				return fieldVal.Int()
			}
			if fieldVal.Kind() >= reflect.Int && fieldVal.Kind() <= reflect.Int64 {
				return fieldVal.Int()
			}
			if fieldVal.Kind() >= reflect.Uint && fieldVal.Kind() <= reflect.Uint64 {
				return int64(fieldVal.Uint())
			}
		}
	}

	return 0
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound)
}

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
		if !errors.Is(err, ErrNotFound) {
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
