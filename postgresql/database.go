package postgresql

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/jonneyless/gbox/logger"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var database *Database

func GetDatabase() *Database {
	return database
}

func DB() *gorm.DB {
	return database.db
}

type DatabaseParams struct {
	Host         string
	Port         int
	Username     string
	Password     string
	Database     string
	Scheme       string
	LogLevel     string
	SSLMode      string
	TimeZone     string
	MaxOpenConns int
	MaxIdleConns int
}

func InitDatabase(c *DatabaseParams) {
	// 设置默认值
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	if c.TimeZone == "" {
		c.TimeZone = "Asia/Shanghai"
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s search_path=\"%s\" port=%d sslmode=%s TimeZone=%s",
		c.Host, c.Username, c.Password, c.Database, c.Scheme, c.Port, c.SSLMode, c.TimeZone)

	database = &Database{
		dsn:          dsn,
		maxOpenConns: c.MaxOpenConns,
		maxIdleConns: c.MaxIdleConns,
		logLevel:     c.LogLevel,
		logger:       logger.GetLogger(),
	}
}

type Database struct {
	dsn          string
	db           *gorm.DB
	maxOpenConns int
	maxIdleConns int
	logLevel     string
	logger       *zap.SugaredLogger
}

func (d *Database) Connect() *gorm.DB {
	var err error

	d.db, err = gorm.Open(postgres.Open(d.dsn), &gorm.Config{
		Logger: logger.NewGormZapLogger(d.logger, d.logLevel),
	})
	if err != nil {
		d.logger.Infoln(d.dsn)
		d.logger.Panicln(err)
	}

	sqlDB, _ := d.db.DB()
	sqlDB.SetMaxOpenConns(max(d.maxOpenConns, 5))
	sqlDB.SetMaxIdleConns(max(d.maxIdleConns, 2))
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	// ✅ 添加回调，在每次查询时打印调用栈
	d.db.Callback().Query().Before("gorm:query").Register("trace_query", func(db *gorm.DB) {
		// 只在包含错误SQL的查询时打印
		if db.Statement.SQL.String() != "" {
			// 打印调用栈
			fmt.Printf("\n=== SQL Trace ===\n")
			fmt.Printf("SQL: %s\n", db.Statement.SQL.String())
			fmt.Printf("Args: %+v\n", db.Statement.Vars)
			fmt.Printf("Stack:\n%s\n", string(debug.Stack()))
			fmt.Printf("=== End Trace ===\n\n")
		}
	})

	return d.db
}
