package main

import (
	"bytes"
	"errors"
	"testing"
)

type failingLogWriter struct{}

func (failingLogWriter) Write([]byte) (int, error) {
	return 0, errors.New("invalid console handle")
}

func TestProgramLogWriterPersistsWhenConsoleIsUnavailable(t *testing.T) {
	var file bytes.Buffer
	writer := programLogWriter{file: &file, console: failingLogWriter{}}

	written, err := writer.Write([]byte("application started\n"))
	if err != nil {
		t.Fatalf("Write returned a console error: %v", err)
	}
	if written != len("application started\n") {
		t.Fatalf("written = %d", written)
	}
	if got := file.String(); got != "application started\n" {
		t.Fatalf("file output = %q", got)
	}
}
