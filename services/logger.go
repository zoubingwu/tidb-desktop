package services

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

var logger *log.Logger

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
	logger = log.New(mw, "", log.LstdFlags)
	return nil
}

func Info(format string, v ...interface{}) {
	if logger != nil {
		logger.Printf("INFO: "+format, v...)
	}
}

func Error(format string, v ...interface{}) error {
	msg := fmt.Sprintf(format, v...)
	if logger != nil {
		logger.Printf("ERROR: %s", msg)
	}
	return fmt.Errorf("%s", msg)
}
