package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LogConfig struct {
	DebugLevel   string
	InfoLevel    string
	ErrorLevel   string
	PanicLevel   string
	FatalLevel   string
	WriteBoth    string
	WriteFile    string
	WriteConsole string
}

var logConfig = LogConfig{
	DebugLevel:   "debug",
	InfoLevel:    "info",
	ErrorLevel:   "error",
	PanicLevel:   "panic",
	FatalLevel:   "fatal",
	WriteBoth:    "both",
	WriteFile:    "file",
	WriteConsole: "console",
}

var zapConfig *ZapConfig
var zapLogger *zap.SugaredLogger

type ZapConfig struct {
	TimeFormat string         `mapstructure:"time_format" json:"time_format" yaml:"time_format" toml:"time_format"`
	Level      string         `mapstructure:"level" json:"level" yaml:"level" toml:"level"`
	Caller     bool           `mapstructure:"caller" json:"caller" yaml:"caller" toml:"caller"`
	StackTrace bool           `mapstructure:"stack_trace" json:"stack_trace" yaml:"stack_trace" toml:"stack_trace"`
	Writer     string         `mapstructure:"writer" json:"writer" yaml:"writer" toml:"writer"`
	Encode     string         `mapstructure:"encode" json:"encode" yaml:"encode" toml:"encode"`
	LogFile    *LogFileConfig `mapstructure:"log_file" json:"log_file" yaml:"log_file" toml:"log_file"`
}

type LogFileConfig struct {
	MaxSize  int      `mapstructure:"max_size" json:"max_size" yaml:"max_size" toml:"max_size"`
	BackUps  int      `mapstructure:"backups" json:"backups" yaml:"backups" toml:"backups"`
	Compress bool     `mapstructure:"compress" json:"compress" yaml:"compress" toml:"compress"`
	Output   []string `mapstructure:"output" json:"output" yaml:"output" toml:"output"`
	Errput   []string `mapstructure:"errput" json:"errput" yaml:"errput" toml:"errput"`
}

func InitLogger(cfg *ZapConfig) *zap.SugaredLogger {
	zapConfig = cfg
	encoder := zapEncoder(cfg)
	levelEnabler := zapLevelEnabler(cfg)
	subCore, options := tee(cfg, encoder, levelEnabler)
	logger := zap.New(subCore, options...)
	zapLogger = logger.Sugar()
	return zapLogger
}

func GetLogger() *zap.SugaredLogger {
	return zapLogger
}

func zapEncoder(cfg *ZapConfig) zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:  "Time",
		LevelKey: "Level",
		NameKey:  "Logger",
		//CallerKey:     "Caller",
		MessageKey:    "Data",
		StacktraceKey: "StackTrace",
		LineEnding:    zapcore.DefaultLineEnding,
		FunctionKey:   zapcore.OmitKey,
	}
	encoderConfig.ConsoleSeparator = " "
	encoderConfig.EncodeTime = timeFormatEncoder
	//encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoderConfig.EncodeLevel = levelEncoder
	encoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	//encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	encoderConfig.EncodeName = zapcore.FullNameEncoder
	switch cfg.Encode {
	case "json":
		{
			return zapcore.NewJSONEncoder(encoderConfig)
		}
	case "console":
		{
			return zapcore.NewConsoleEncoder(encoderConfig)
		}
	}

	return zapcore.NewConsoleEncoder(encoderConfig)
}

func zapLevelEnabler(cfg *ZapConfig) zapcore.LevelEnabler {
	switch cfg.Level {
	case logConfig.DebugLevel:
		return zap.DebugLevel
	case logConfig.InfoLevel:
		return zap.InfoLevel
	case logConfig.ErrorLevel:
		return zap.ErrorLevel
	case logConfig.PanicLevel:
		return zap.PanicLevel
	case logConfig.FatalLevel:
		return zap.FatalLevel
	}

	return zap.DebugLevel
}

func zapWriteSyncer(cfg *ZapConfig) zapcore.WriteSyncer {
	syncers := make([]zapcore.WriteSyncer, 0, 2)
	// 如果开启了日志控制台输出，就加入控制台书写器
	if cfg.Writer == logConfig.WriteBoth || cfg.Writer == logConfig.WriteConsole {
		syncers = append(syncers, zapcore.AddSync(os.Stdout))
	}

	// 如果开启了日志文件存储，就根据文件路径切片加入书写器
	if cfg.Writer == logConfig.WriteBoth || cfg.Writer == logConfig.WriteFile {
		// 添加日志输出器
		for _, path := range cfg.LogFile.Output {
			logger := &lumberjack.Logger{
				Filename:   path,                 //文件路径
				MaxSize:    cfg.LogFile.MaxSize,  //分割文件的大小
				MaxBackups: cfg.LogFile.BackUps,  //备份次数
				Compress:   cfg.LogFile.Compress, // 是否压缩
				LocalTime:  true,                 //使用本地时间
			}
			syncers = append(syncers, zapcore.Lock(zapcore.AddSync(logger)))
		}
	}
	return zap.CombineWriteSyncers(syncers...)
}

func tee(cfg *ZapConfig, encoder zapcore.Encoder, levelEnabler zapcore.LevelEnabler) (core zapcore.Core, options []zap.Option) {
	sink := zapWriteSyncer(cfg)
	return zapcore.NewCore(encoder, sink, levelEnabler), buildOptions(cfg, levelEnabler)
}

func buildOptions(cfg *ZapConfig, levelEnabler zapcore.LevelEnabler) (options []zap.Option) {
	if cfg.Caller {
		options = append(options, zap.AddCaller())
	}

	if cfg.StackTrace {
		options = append(options, zap.AddStacktrace(levelEnabler))
	}
	return
}

func timeFormatEncoder(t time.Time, encoder zapcore.PrimitiveArrayEncoder) {
	encoder.AppendString(t.Format(zapConfig.TimeFormat))
}

func levelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var colorCode string
	var levelStr string

	switch level {
	case zapcore.DebugLevel:
		colorCode = "\033[36m" // 青色
		levelStr = "DEBUG"
	case zapcore.InfoLevel:
		colorCode = "\033[32m" // 绿色
		levelStr = "INFO "
	case zapcore.WarnLevel:
		colorCode = "\033[33m" // 黄色
		levelStr = "WARN "
	case zapcore.ErrorLevel:
		colorCode = "\033[31m" // 红色
		levelStr = "ERROR"
	case zapcore.DPanicLevel:
		colorCode = "\033[35m" // 紫色
		levelStr = "DPANIC"
	case zapcore.PanicLevel:
		colorCode = "\033[35m" // 紫色
		levelStr = "PANIC"
	case zapcore.FatalLevel:
		colorCode = "\033[31m" // 红色
		levelStr = "FATAL"
	default:
		colorCode = "\033[0m" // 重置
		levelStr = fmt.Sprintf("%-5s", level.String())
	}

	enc.AppendString(fmt.Sprintf("%s%-5s\033[0m", colorCode, levelStr))
}
