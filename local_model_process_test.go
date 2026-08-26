package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWorkerExitErrorIncludesStderr(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestWorkerExitErrorHelperProcess")
	cmd.Env = append(os.Environ(), "QUICK_TRANSLATION_WORKER_EXIT_HELPER=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	service := &localModelService{
		cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), stderr: stderr, loadedModel: "test-model",
	}
	_, readErr := service.stdout.ReadBytes('\n')
	if readErr == nil {
		t.Fatal("expected stdout EOF")
	}
	result := service.workerExitError(readErr)
	if !strings.Contains(result.Error(), "具体的 Python 错误") {
		t.Fatalf("error = %q; stderr detail is missing", result)
	}
	if service.cmd != nil || service.stdin != nil || service.stdout != nil || service.stderr != nil || service.loadedModel != "" {
		t.Fatal("worker state was not reset")
	}
}

func TestWorkerExitErrorHelperProcess(t *testing.T) {
	if os.Getenv("QUICK_TRANSLATION_WORKER_EXIT_HELPER") != "1" {
		return
	}
	_, _ = io.WriteString(os.Stderr, "具体的 Python 错误")
	os.Exit(7)
}
