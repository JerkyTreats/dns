// Package logging provides centralized logging for PHITE using uber-go/zap.
// All code must use this package for logging.
package logging

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jerkytreats/dns/internal/config"
	"go.uber.org/zap"
)

var (
	logger     *zap.SugaredLogger
	loggerOnce sync.Once
)

// getZapLevel maps config log_level string to zapcore.Level.
// Supports 'NONE' to silence all logs (for testing).
func getZapLevel() zap.AtomicLevel {
	levelStr := strings.ToUpper(config.GetString("log_level"))
	switch levelStr {
	case "DEBUG":
		return zap.NewAtomicLevelAt(zap.DebugLevel)
	case "ERROR":
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	case "WARN":
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	case "NONE":
		// Use zapcore.FatalLevel+1 to silence all logs
		return zap.NewAtomicLevelAt(100) // higher than FatalLevel
	case "INFO":
		fallthrough
	default:
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	}
}

// initLogger initializes the zap logger singleton.
func initLogger() {
	loggerOnce.Do(func() {
		cfg := zap.NewDevelopmentConfig()
		cfg.Level = getZapLevel()
		l, err := cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
		if err != nil {
			panic(fmt.Sprintf("failed to build logger: %v", err))
		}
		logger = l.Sugar()
	})
}

// Info logs an info-level message.
func Info(format string, args ...interface{}) {
	initLogger()
	logger.Infof(format, args...)
}

// Debug logs a debug-level message.
func Debug(format string, args ...interface{}) {
	initLogger()
	logger.Debugf(format, args...)
}

// Error logs an error-level message.
func Error(format string, args ...interface{}) {
	initLogger()
	logger.Errorf(format, args...)
}

// Warn logs a warning-level message.
func Warn(format string, args ...interface{}) {
	initLogger()
	logger.Warnf(format, args...)
}

// Sync flushes any buffered log entries.
func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

// For testing: resetLogger resets the logger singleton.
func resetLogger() {
	logger = nil
	loggerOnce = sync.Once{}
}
