package mysql

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jonneyless/gbox/cache"
	"github.com/shopspring/decimal"
	"github.com/spf13/cast"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DefaultCacheTTL = time.Hour
	MaxCacheTTL     = 24 * time.Hour
)

type BaseService[T any] struct {
	Prefix    string
	TableName string
	readOnly  bool
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
	Expression *clause.Expr
}

func WithOrder(orderBy string) QueryOption {
	return func(o *QueryOptions) {
		o.Orders = append(o.Orders, QueryOrder{
			OrderBy: orderBy,
		})
	}
}

func WithOrderDesc(orderBy string) QueryOption {
	return func(o *QueryOptions) {
		o.Orders = append(o.Orders, QueryOrder{
			OrderBy:    orderBy,
			Descending: true,
		})
	}
}

func WithOrderExpr(orderBy string) QueryOption {
	return func(o *QueryOptions) {
		o.Orders = append(o.Orders, QueryOrder{
			Expression: new(gorm.Expr(orderBy)),
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

func (srv *BaseService[T]) ReadOnly() *BaseService[T] {
	if DBRead() != nil {
		srv.readOnly = true
	}
	return srv
}

func (srv *BaseService[T]) getDB() *gorm.DB {
	if srv.readOnly {
		srv.readOnly = false
		return DBRead()
	}

	return DB()
}

func (srv *BaseService[T]) WithTransaction(fn func(tx *gorm.DB) error) error {
	return DB().Transaction(fn)
}

func (srv *BaseService[T]) GetById(id int64) (*T, error) {
	var model *T

	err := srv.getDB().Where("id = ?", id).First(&model).Error
	if err != nil {
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
			return nil, gorm.ErrRecordNotFound
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
		return nil, gorm.ErrRecordNotFound
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
		return nil, fmt.Errorf("no conditions provided")
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
			if order.Expression != nil {
				query = query.Order(order.Expression)
			} else {
				if order.Descending {
					query = query.Order(order.OrderBy + " DESC")
				} else {
					query = query.Order(order.OrderBy + " ASC")
				}
			}
		}
	}

	err := query.First(&model).Error
	if err != nil {
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
		return nil, fmt.Errorf("no conditions provided")
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
			if order.Expression != nil {
				query = query.Order(order.Expression)
			} else {
				if order.Descending {
					query = query.Order(order.OrderBy + " DESC")
				} else {
					query = query.Order(order.OrderBy + " ASC")
				}
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
	err := srv.getDB().Model(new(T)).Find(&models).Error
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
		return fmt.Errorf("no models provided")
	}

	if batchSize <= 0 {
		return fmt.Errorf("batch size must be greater than zero")
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

// UpdateJson MySQL 版本：使用 JSON_SET 和 JSON_REMOVE 操作 JSON 字段
func (srv *BaseService[T]) UpdateJson(id int64, field string, jsonKey string, value any) error {
	var err error
	if value == nil {
		// 移除 JSON 键
		sql := fmt.Sprintf("UPDATE %s SET %s = JSON_REMOVE(%s, '$.\"%s\"') WHERE id = ?",
			srv.TableName, field, field, jsonKey)
		err = DB().Exec(sql, id).Error
	} else {
		// 设置 JSON 键值
		sql := fmt.Sprintf("UPDATE %s SET %s = JSON_SET(%s, '$.\"%s\"', ?) WHERE id = ?",
			srv.TableName, field, field, jsonKey)
		err = DB().Exec(sql, value, id).Error
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
		return fmt.Errorf("no updates provided")
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
		return 0, fmt.Errorf("no conditions provided")
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
		return fmt.Errorf("no ids provided")
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
		return 0, fmt.Errorf("no conditions provided")
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
		return 0, fmt.Errorf("no conditions provided")
	}

	var count int64

	err := srv.buildQuery(conditions).Count(&count).Error
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (srv *BaseService[T]) Raw(result any, sql string, args ...any) error {
	return srv.getDB().Raw(sql, args...).Scan(result).Error
}

func (srv *BaseService[T]) Scan(result any, conditions []Condition, opts ...QueryOption) error {
	options := &QueryOptions{
		Select: []string{},
	}
	for _, opt := range opts {
		opt(options)
	}

	query := srv.buildQuery(conditions)

	if len(options.Select) > 0 {
		query = query.Select(options.Select)
	}

	return query.Scan(result).Error
}

func (srv *BaseService[T]) SumInt64(column string, conditions []Condition) (int64, error) {
	var sum int64
	err := srv.buildQuery(conditions).Select(fmt.Sprintf("SUM(%s)", column)).Scan(&sum).Error
	if err != nil {
		return 0, err
	}

	return sum, nil
}

func (srv *BaseService[T]) SumFloat64(column string, conditions []Condition) (float64, error) {
	var sum float64
	err := srv.buildQuery(conditions).Select(fmt.Sprintf("SUM(%s)", column)).Scan(&sum).Error
	if err != nil {
		return 0, err
	}

	return sum, nil
}

func (srv *BaseService[T]) SumDecimal(column string, conditions []Condition) (decimal.Decimal, error) {
	var sum decimal.Decimal
	err := srv.buildQuery(conditions).Select(fmt.Sprintf("SUM(%s)", column)).Scan(&sum).Error
	if err != nil {
		return decimal.Zero, err
	}

	return sum, nil
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
		case "JSON_EXTRACT", "JSON":
			// MySQL JSON 查询：支持 JSON_EXTRACT 或 -> 操作符
			fields := strings.Split(cond.Field, ".")
			if len(fields) == 1 {
				query = query.Where(fmt.Sprintf("JSON_EXTRACT(%s, '$.\"%s\"') = ?", cond.Field, cast.ToString(cond.Value)), cond.Value)
			} else if len(fields) == 2 {
				query = query.Where(fmt.Sprintf("JSON_EXTRACT(%s, '$.\"%s\"') = ?", fields[0], fields[1]), cond.Value)
			} else if len(fields) == 3 {
				query = query.Where(fmt.Sprintf("JSON_EXTRACT(%s, '$.\"%s\".\"%s\"') = ?", fields[0], fields[1], fields[2]), cond.Value)
			}
		default:
			query = query.Where(fmt.Sprintf("%s = ?", cond.Field), cond.Value)
		}
	}

	return query
}

func (srv *BaseService[T]) buildQuery(conditions []Condition) *gorm.DB {
	return srv.buildQueryCore(srv.getDB(), conditions)
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
