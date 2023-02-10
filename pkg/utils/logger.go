package utils

import (
	"log"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger     *zap.SugaredLogger
	onceLogger sync.Once
)

func zapLogLevel(level string) zap.AtomicLevel {
	switch level {
	case "debug", "DEBUG":
		return zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info", "INFO", "": // make the zero value useful
		return zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn", "WARN":
		return zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error", "ERROR":
		return zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	case "dpanic", "DPANIC":
		return zap.NewAtomicLevelAt(zapcore.DPanicLevel)
	case "panic", "PANIC":
		return zap.NewAtomicLevelAt(zapcore.PanicLevel)
	case "fatal", "FATAL":
		return zap.NewAtomicLevelAt(zapcore.FatalLevel)
	default:
		return zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
}

func GetLogger() *zap.SugaredLogger {
	// Read log level from the environment
	logLevel := GetEnv("LOG_LEVEL", "INFO")
	onceLogger.Do(
		func() {

			var zapConfig = zap.NewProductionConfig()
			zapConfig.InitialFields = map[string]interface{}{
				"SERVICE": "ecr-creds-rotation",
			}
			zapConfig.OutputPaths = []string{"stdout"}
			zapConfig.ErrorOutputPaths = []string{"stdout"}
			zapConfig.EncoderConfig.TimeKey = "@timestamp"
			zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
			zapConfig.Level = zapLogLevel(logLevel)
			zapLogger, err := zapConfig.Build()
			if err != nil {
				log.Fatalf("Exiting! Unable to create a logger")
			}
			logger = zapLogger.Sugar()
		})

	return logger
}
