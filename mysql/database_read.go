package mysql

import (
	"fmt"

	"github.com/jonneyless/gbox/logger"

	"gorm.io/gorm"
)

var databaseRead *Database

func GetDatabaseRead() *Database {
	return databaseRead
}

func DBRead() *gorm.DB {
	if databaseRead == nil {
		return nil
	}

	return databaseRead.db
}

func InitDatabaseRead(c *DatabaseParams) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", c.Username, c.Password, c.Host, c.Port, c.Database)

	databaseRead = &Database{
		dsn:          dsn,
		maxOpenConns: c.MaxOpenConns,
		maxIdleConns: c.MaxIdleConns,
		logLevel:     c.LogLevel,
		logger:       logger.GetLogger(),
	}
}
