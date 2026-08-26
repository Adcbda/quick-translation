package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

func initialiseProgramLog() (*os.File, string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, "", err
	}
	logDir := filepath.Join(configDir, "QuickTranslation", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, "", err
	}
	logPath := filepath.Join(logDir, "quick-translation.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, logPath, err
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	return file, logPath, nil
}
