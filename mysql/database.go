package mysql

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
	LogLevel     string
	MaxOpenConns int
	MaxIdleConns int
}

func InitDatabase(c *DatabaseParams) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", c.Username, c.Password, c.Host, c.Port, c.Database)

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

	return d.db
}
