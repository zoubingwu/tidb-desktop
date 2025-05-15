package services

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/logger"
)

// appLogger is an implementation of the wails logger interface.
type appLogger struct {
	logger *log.Logger
}

// Global logger instance
var GlobalLogger logger.Logger

// InitLogger initializes the application logger.
func InitLogger() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	logDir := filepath.Join(homeDir, ConfigDirName)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return fmt.Errorf("failed to create log directory: %v", err)
	}

	logFile := filepath.Join(logDir, "tidb-desktop.log")
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	mw := io.MultiWriter(os.Stderr, f)
	internalLogger := log.New(mw, "", log.LstdFlags)
	GlobalLogger = &appLogger{logger: internalLogger}
	return nil
}

func (l *appLogger) Print(message string) {
	l.logger.Print(message)
}

func (l *appLogger) Trace(message string) {
	l.logger.Printf("TRACE: %s", message)
}

func (l *appLogger) Debug(message string) {
	l.logger.Printf("DEBUG: %s", message)
}

func (l *appLogger) Info(message string) {
	l.logger.Printf("INFO: %s", message)
}

func (l *appLogger) Warning(message string) {
	l.logger.Printf("WARNING: %s", message)
}

func (l *appLogger) Error(message string) {
	l.logger.Printf("ERROR: %s", message)
}

func (l *appLogger) Fatal(message string) {
	l.logger.Fatalf("FATAL: %s", message)
}

func LogInfo(format string, v ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Info(fmt.Sprintf(format, v...))
	}
}

func LogError(format string, v ...interface{}) error {
	msg := fmt.Sprintf(format, v...)
	if GlobalLogger != nil {
		GlobalLogger.Error(msg)
	}
	return fmt.Errorf("%s", msg)
}

func LogDebug(format string, v ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Debug(fmt.Sprintf(format, v...))
	}
}

func LogWarning(format string, v ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Warning(fmt.Sprintf(format, v...))
	}
}

func LogFatal(format string, v ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Fatal(fmt.Sprintf(format, v...))
	} else {
		log.Fatalf("FATAL: "+format, v...) // Fallback if logger not initialized
	}
}
