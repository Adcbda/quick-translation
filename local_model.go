package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed python/translator_worker.py
var workerScript []byte

//go:embed python/requirements.txt
var requirements []byte

type localModelService struct {
	mu          sync.Mutex
	baseDir     string
	modelsDir   string
	runtimeDir  string
	python      string
	pythonArgs  []string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Reader
	stderr      *bytes.Buffer
	loadedModel string
	nextID      int64
}

type workerRequest struct {
	ID     int64  `json:"id"`
	Text   string `json:"text"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type workerResponse struct {
	ID    int64  `json:"id"`
	Text  string `json:"text"`
	Error string `json:"error"`
}

type modelDownloadProgress struct {
	Message string
	Percent int
}

var (
	ansiEscapePattern    = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	namedProgressPattern = regexp.MustCompile(`^(.+?):\s*(\d{1,3})%\|`)
	progressPattern      = regexp.MustCompile(`(?:^|\s)(\d{1,3})%`)
)

func newLocalModelService() (*localModelService, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(configDir, "QuickTranslation")
	runtimeDir := filepath.Join(base, "runtime")
	modelsDir := filepath.Join(base, "models")
	for _, dir := range []string{runtimeDir, modelsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	if err := writeIfChanged(filepath.Join(runtimeDir, "translator_worker.py"), workerScript); err != nil {
		return nil, err
	}
	if err := writeIfChanged(filepath.Join(runtimeDir, "requirements.txt"), requirements); err != nil {
		return nil, err
	}
	python, args := findPython()
	log.Printf("local model service ready; base=%s python=%s", base, python)
	return &localModelService{
		baseDir: base, modelsDir: modelsDir, runtimeDir: runtimeDir,
		python: python, pythonArgs: args,
	}, nil
}

func writeIfChanged(path string, content []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return nil
	}
	return os.WriteFile(path, content, 0o644)
}

func findPython() (string, []string) {
	candidates := []struct {
		name string
		args []string
	}{
		{"python3", nil},
		{"python", nil},
	}
	if runtime.GOOS == "windows" {
		candidates = []struct {
			name string
			args []string
		}{{"py", []string{"-3"}}, {"python", nil}, {"python3", nil}}
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			continue
		}
		args := append(append([]string{}, candidate.args...), "--version")
		if pythonCommand(path, args...).Run() == nil {
			return path, candidate.args
		}
	}
	return "", nil
}

func (s *localModelService) RuntimeStatus() RuntimeStatus {
	if s.python == "" {
		return RuntimeStatus{Message: "未找到 Python 3，请先安装 Python 3.10 或更高版本"}
	}
	args := append(append([]string{}, s.pythonArgs...), "-c", "import torch, transformers, sentencepiece")
	err := pythonCommand(s.python, args...).Run()
	if err != nil {
		return RuntimeStatus{PythonFound: true, Python: s.python, Message: "需要安装 PyTorch、Transformers 和 SentencePiece"}
	}
	return RuntimeStatus{PythonFound: true, Ready: true, Python: s.python, Message: "本地翻译运行环境已就绪"}
}

func (s *localModelService) PrepareRuntime(onProgress func(string)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.python == "" {
		return errors.New("未找到 Python 3；请安装 Python 3.10 或更高版本后重试")
	}
	if onProgress != nil {
		onProgress("正在安装 PyTorch 与 Transformers 依赖…")
	}
	args := append(append([]string{}, s.pythonArgs...), "-m", "pip", "install", "-r", filepath.Join(s.runtimeDir, "requirements.txt"))
	cmd := pythonCommand(s.python, args...)
	output, err := cmd.CombinedOutput()
	if text := strings.TrimSpace(string(output)); text != "" {
		log.Printf("runtime installer output:\n%s", text)
	}
	if err != nil {
		message := tail(string(output), 1600)
		return fmt.Errorf("运行环境安装失败: %w\n%s", err, message)
	}
	return nil
}

func (s *localModelService) Download(modelID string, onProgress func(modelDownloadProgress)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.python == "" {
		return errors.New("未找到 Python 3")
	}
	if !s.RuntimeStatusUnlocked().Ready {
		return errors.New("本地运行环境未就绪，请先点击“准备运行环境”")
	}
	args := append(append([]string{}, s.pythonArgs...), filepath.Join(s.runtimeDir, "translator_worker.py"),
		"--model", modelID, "--cache-dir", s.modelsDir, "--download-only")
	cmd := pythonCommand(s.python, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var output bytes.Buffer
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(io.TeeReader(stderr, &output))
		scanner.Split(scanLinesAndCarriageReturns)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		lastKey := ""
		for scanner.Scan() {
			line := cleanProgressLine(scanner.Text())
			if line == "" {
				continue
			}
			progress := parseModelDownloadProgress(line)
			key := fmt.Sprintf("%d:%s", progress.Percent, progress.Message)
			if key == lastKey {
				continue
			}
			lastKey = key
			log.Printf("model download output; model=%s progress=%d message=%s", modelID, progress.Percent, line)
			if onProgress != nil {
				onProgress(progress)
			}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("reading model download output failed; model=%s error=%v", modelID, err)
		}
		close(done)
	}()
	stdoutData, _ := io.ReadAll(stdout)
	if text := strings.TrimSpace(string(stdoutData)); text != "" {
		log.Printf("model download stdout; model=%s output=%s", modelID, text)
	}
	err = cmd.Wait()
	<-done
	if err != nil {
		return fmt.Errorf("模型下载失败: %w\n%s%s", err, tail(output.String(), 1200), tail(string(stdoutData), 500))
	}
	if err := os.MkdirAll(s.readyDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.markerPath(modelID), []byte(time.Now().Format(time.RFC3339)), 0o644)
}

func (s *localModelService) RuntimeStatusUnlocked() RuntimeStatus {
	if s.python == "" {
		return RuntimeStatus{Message: "未找到 Python 3"}
	}
	args := append(append([]string{}, s.pythonArgs...), "-c", "import torch, transformers, sentencepiece")
	if err := pythonCommand(s.python, args...).Run(); err != nil {
		return RuntimeStatus{PythonFound: true, Python: s.python, Message: "依赖未安装"}
	}
	return RuntimeStatus{PythonFound: true, Ready: true, Python: s.python, Message: "已就绪"}
}

func (s *localModelService) readyDir() string {
	return filepath.Join(s.baseDir, "ready")
}

func safeModelName(modelID string) string {
	return strings.NewReplacer("/", "--", "\\", "--", ":", "-").Replace(modelID)
}

func (s *localModelService) markerPath(modelID string) string {
	return filepath.Join(s.readyDir(), safeModelName(modelID)+".ready")
}

func (s *localModelService) IsDownloaded(modelID string) bool {
	_, err := os.Stat(s.markerPath(modelID))
	return err == nil
}

func (s *localModelService) Translate(modelID string, request TranslateRequest) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.IsDownloaded(modelID) {
		return "", errors.New("所选本地模型尚未下载，请先到模型管理中下载")
	}
	if err := s.ensureWorker(modelID); err != nil {
		return "", err
	}
	s.nextID++
	req := workerRequest{ID: s.nextID, Text: request.Text, Source: request.Source, Target: request.Target}
	data, _ := json.Marshal(req)
	if _, err := s.stdin.Write(append(data, '\n')); err != nil {
		s.closeUnlocked()
		return "", fmt.Errorf("写入本地模型失败: %w", err)
	}
	line, err := s.stdout.ReadBytes('\n')
	if err != nil {
		return "", s.workerExitError(err)
	}
	var response workerResponse
	if err := json.Unmarshal(line, &response); err != nil {
		s.closeUnlocked()
		return "", fmt.Errorf("本地模型响应无效: %w（响应内容: %s）", err, shorten(strings.TrimSpace(string(line)), 300))
	}
	if response.Error != "" {
		return "", errors.New(response.Error)
	}
	return strings.TrimSpace(response.Text), nil
}

func (s *localModelService) ensureWorker(modelID string) error {
	if s.cmd != nil && s.loadedModel == modelID {
		return nil
	}
	s.closeUnlocked()
	if !s.RuntimeStatusUnlocked().Ready {
		return errors.New("本地翻译运行环境未就绪，请在配置页准备运行环境")
	}
	args := append(append([]string{}, s.pythonArgs...), filepath.Join(s.runtimeDir, "translator_worker.py"),
		"--model", modelID, "--cache-dir", s.modelsDir)
	cmd := pythonCommand(s.python, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = io.MultiWriter(stderr, log.Writer())
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReader(stdout)
	s.stderr = stderr
	s.loadedModel = modelID
	return nil
}

func pythonCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	// Python otherwise uses the active Windows code page for redirected streams.
	// That can terminate the worker with UnicodeEncodeError as soon as it returns
	// a translated string containing Chinese or another non-ASCII script.
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	configureHiddenProcess(cmd)
	return cmd
}

func (s *localModelService) workerExitError(readErr error) error {
	cmd := s.cmd
	stderr := s.stderr
	var waitErr error
	if cmd != nil {
		// EOF means the worker closed stdout. Wait for stderr forwarding to finish
		// so the useful Python traceback is included in the returned error.
		waitErr = cmd.Wait()
	}
	detail := ""
	if stderr != nil {
		detail = tail(stderr.String(), 1600)
	}
	s.resetWorkerUnlocked()
	if detail != "" {
		return fmt.Errorf("本地模型进程异常退出: %s", detail)
	}
	if waitErr != nil {
		return fmt.Errorf("本地模型进程异常退出: %v", waitErr)
	}
	return fmt.Errorf("本地模型进程异常退出: %w", readErr)
}

func scanLinesAndCarriageReturns(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for index, value := range data {
		if value != '\r' && value != '\n' {
			continue
		}
		advance = index + 1
		if value == '\r' && len(data) > index+1 && data[index+1] == '\n' {
			advance++
		}
		return advance, data[:index], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func cleanProgressLine(value string) string {
	return strings.TrimSpace(ansiEscapePattern.ReplaceAllString(value, ""))
}

func parseModelDownloadProgress(line string) modelDownloadProgress {
	progress := modelDownloadProgress{Message: shorten(line, 180), Percent: -1}
	if match := namedProgressPattern.FindStringSubmatch(line); len(match) == 3 {
		percent, err := strconv.Atoi(match[2])
		if err == nil && percent >= 0 && percent <= 100 {
			name := shorten(strings.TrimSpace(match[1]), 80)
			progress.Percent = percent
			progress.Message = fmt.Sprintf("正在下载 %s · 当前文件 %d%%", name, percent)
		}
		return progress
	}
	if match := progressPattern.FindStringSubmatch(line); len(match) == 2 {
		percent, err := strconv.Atoi(match[1])
		if err == nil && percent >= 0 && percent <= 100 {
			progress.Percent = percent
			progress.Message = fmt.Sprintf("正在下载模型文件 · 当前文件 %d%%", percent)
		}
	}
	return progress
}

func (s *localModelService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeUnlocked()
}

func (s *localModelService) closeUnlocked() {
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
	s.resetWorkerUnlocked()
}

func (s *localModelService) resetWorkerUnlocked() {
	s.cmd = nil
	s.stdin = nil
	s.stdout = nil
	s.stderr = nil
	s.loadedModel = ""
}

func shorten(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

func tail(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return "…" + string(runes[len(runes)-max:])
}
