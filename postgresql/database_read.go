package postgresql

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
	// 设置默认值
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	if c.TimeZone == "" {
		c.TimeZone = "Asia/Shanghai"
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s search_path=\"%s\" port=%d sslmode=%s TimeZone=%s",
		c.Host, c.Username, c.Password, c.Database, c.Scheme, c.Port, c.SSLMode, c.TimeZone)

	databaseRead = &Database{
		dsn:          dsn,
		maxOpenConns: c.MaxOpenConns,
		maxIdleConns: c.MaxIdleConns,
		logLevel:     c.LogLevel,
		logger:       logger.GetLogger(),
	}
}
