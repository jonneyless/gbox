package postgresql

import (
	"fmt"
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

	// 调试用代码
	//d.db.Callback().Query().After("gorm:query").Register("trace_after_query", func(db *gorm.DB) {
	//	sql := db.Statement.SQL.String()
	//	if sql == "" {
	//		return
	//	}
	//
	//	d.logger.Debugw("QUERY EXECUTED",
	//		"sql", sql,
	//		"args", fmt.Sprintf("%+v", db.Statement.Vars), // 直接格式化
	//		"args_len", len(db.Statement.Vars),
	//		"rows_affected", db.RowsAffected,
	//	)
	//
	//	for i, arg := range db.Statement.Vars {
	//		d.logger.Debugw(fmt.Sprintf("  Arg[%d]", i),
	//			"value", arg,
	//			"type", fmt.Sprintf("%T", arg),
	//		)
	//	}
	//
	//	stack := string(debug.Stack())
	//	d.logger.Debugw("Full stack trace", "stack", stack)
	//})

	sqlDB, _ := d.db.DB()
	sqlDB.SetMaxOpenConns(max(d.maxOpenConns, 5))
	sqlDB.SetMaxIdleConns(max(d.maxIdleConns, 2))
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	return d.db
}
