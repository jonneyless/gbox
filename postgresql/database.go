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
	Host     string
	Port     int
	Username string
	Password string
	Database string
	Scheme   string
	LogLevel string
	SSLMode  string
	TimeZone string
}

func InitDatabase(c *DatabaseParams) {
	// 设置默认值
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	if c.TimeZone == "" {
		c.TimeZone = "Asia/Shanghai"
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s search_path=%s port=%d sslmode=%s TimeZone=%s",
		c.Host, c.Username, c.Password, c.Database, c.Scheme, c.Port, c.SSLMode, c.TimeZone)

	logger.GetLogger().Debugln(dsn)
	database = &Database{dsn: dsn, logLevel: c.LogLevel, logger: logger.GetLogger()}
}

type Database struct {
	dsn      string
	db       *gorm.DB
	logLevel string
	logger   *zap.SugaredLogger
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
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(time.Hour)

	return d.db
}
