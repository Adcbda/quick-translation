package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// programLogWriter treats the log file as the durable destination. Windows
// GUI executables launched outside a terminal can have an invalid stderr
// handle, so a console write failure must not prevent file logging.
type programLogWriter struct {
	file    io.Writer
	console io.Writer
}

func (w programLogWriter) Write(data []byte) (int, error) {
	written, err := w.file.Write(data)
	if err != nil {
		return written, err
	}
	if w.console != nil {
		_, _ = w.console.Write(data)
	}
	return len(data), nil
}

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
	log.SetOutput(programLogWriter{file: file, console: os.Stderr})
	return file, logPath, nil
}
