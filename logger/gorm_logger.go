package logger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type LevelMap struct {
	SilentLevel string
	ErrorLevel  string
	WarnLevel   string
	InfoLevel   string
}

var LevelMaps = LevelMap{
	SilentLevel: "silent",
	ErrorLevel:  "error",
	WarnLevel:   "warn",
	InfoLevel:   "info",
}

type GormZapLogger struct {
	zapLogger     *zap.SugaredLogger
	LogLevel      logger.LogLevel
	SlowThreshold time.Duration
}

// NewGormZapLogger 创建新的 GORM zap logger
func NewGormZapLogger(zapLogger *zap.SugaredLogger, logLevel string) *GormZapLogger {
	level := logger.Error

	switch logLevel {
	case LevelMaps.SilentLevel:
		level = logger.Silent
	case LevelMaps.ErrorLevel:
		level = logger.Error
	case LevelMaps.WarnLevel:
		level = logger.Warn
	case LevelMaps.InfoLevel:
		level = logger.Info
	}

	return &GormZapLogger{
		zapLogger:     zapLogger,
		LogLevel:      level,
		SlowThreshold: time.Second, // 默认慢查询阈值 1 秒
	}
}

// WithSlowThreshold 设置慢查询阈值
func (l *GormZapLogger) WithSlowThreshold(threshold time.Duration) *GormZapLogger {
	l.SlowThreshold = threshold
	return l
}

// LogMode 实现 logger.Interface 的 LogMode 方法
func (l *GormZapLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

// Info 实现 logger.Interface 的 Info 方法
func (l *GormZapLogger) Info(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Info {
		l.zapLogger.Infow(msg, "caller", l.getCaller(), "data", fmt.Sprint(data...))
	}
}

// Warn 实现 logger.Interface 的 Warn 方法
func (l *GormZapLogger) Warn(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Warn {
		l.zapLogger.Warnw(msg, "caller", l.getCaller(), "data", fmt.Sprint(data...))
	}
}

// Error 实现 logger.Interface 的 Error 方法
func (l *GormZapLogger) Error(ctx context.Context, msg string, data ...any) {
	if l.LogLevel >= logger.Error {
		l.zapLogger.Errorw(msg, "caller", l.getCaller(), "data", fmt.Sprint(data...))
	}
}

// Trace 实现 logger.Interface 的 Trace 方法
func (l *GormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	// 日志字段
	fields := []any{
		"caller", l.getCaller(),
		"elapsed", elapsed,
		"rows", rows,
		"sql", sql,
	}

	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && l.LogLevel >= logger.Error:
		l.zapLogger.Errorf("SQL执行失败：%s -- %s", err.Error(), sql)
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0 && l.LogLevel >= logger.Warn:
		l.zapLogger.Warnw("Slow SQL", append(fields, "threshold", l.SlowThreshold)...)
	case l.LogLevel >= logger.Info:
		l.zapLogger.Infof("SQl Exec [Rows %d] %s (耗时 %s)", rows, sql, elapsed)
	}
}

// getCaller 获取调用者信息
func (l *GormZapLogger) getCaller() string {
	// 跳过 4 层调用栈，找到实际的调用者
	_, file, line, ok := runtime.Caller(4)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}
