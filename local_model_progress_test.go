package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestScanLinesAndCarriageReturns(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("model.safetensors: 12%|x\rmodel.safetensors: 13%|x\r\nwarning\nfinished"))
	scanner.Split(scanLinesAndCarriageReturns)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"model.safetensors: 12%|x", "model.safetensors: 13%|x", "warning", "finished"}
	if len(lines) != len(want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
	for index := range want {
		if lines[index] != want[index] {
			t.Fatalf("line %d: got %q, want %q", index, lines[index], want[index])
		}
	}
}

func TestParseNamedModelDownloadProgress(t *testing.T) {
	progress := parseModelDownloadProgress("model.safetensors: 42%|████ | 420M/1.0G")
	if progress.Percent != 42 {
		t.Fatalf("got percent %d, want 42", progress.Percent)
	}
	if progress.Message != "正在下载 model.safetensors · 当前文件 42%" {
		t.Fatalf("unexpected message %q", progress.Message)
	}
}

func TestParseModelDownloadWarning(t *testing.T) {
	line := "Xet Storage is enabled, falling back to regular HTTP download"
	progress := parseModelDownloadProgress(line)
	if progress.Percent != -1 {
		t.Fatalf("got percent %d, want -1", progress.Percent)
	}
	if progress.Message != line {
		t.Fatalf("got message %q, want %q", progress.Message, line)
	}
}
