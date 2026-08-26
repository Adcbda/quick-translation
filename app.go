package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type TranslateRequest struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type TranslateResult struct {
	Text     string `json:"text"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Size        string `json:"size"`
	Downloaded  bool   `json:"downloaded"`
	Selected    bool   `json:"selected"`
	Default     bool   `json:"default"`
}

type RuntimeStatus struct {
	PythonFound bool   `json:"pythonFound"`
	Ready       bool   `json:"ready"`
	Python      string `json:"python"`
	Message     string `json:"message"`
}

type ModelDownloadStatus struct {
	Model     string `json:"model"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Progress  int    `json:"progress"`
	UpdatedAt string `json:"updatedAt"`
}

type App struct {
	ctx            context.Context
	store          *configStore
	local          *localModelService
	shortcut       *shortcutListener
	mu             sync.Mutex
	downloadMu     sync.RWMutex
	downloadStatus ModelDownloadStatus
	logFile        *os.File
	logPath        string
	initErr        error
}

func NewApp() *App {
	app := &App{downloadStatus: ModelDownloadStatus{Status: "idle", Progress: -1}}
	logFile, logPath, logErr := initialiseProgramLog()
	app.logFile = logFile
	app.logPath = logPath
	if logErr != nil {
		log.Printf("unable to initialise file logging: %v", logErr)
	} else {
		log.Printf("application starting; log=%s", logPath)
	}
	store, err := newConfigStore()
	if err != nil {
		app.initErr = err
		log.Printf("configuration initialisation failed: %v", err)
		return app
	}
	local, localErr := newLocalModelService()
	if localErr != nil {
		err = localErr
		log.Printf("local model service initialisation failed: %v", localErr)
	}
	app.store = store
	app.local = local
	app.initErr = err
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Printf("application window started")
	if a.initErr != nil || a.store == nil {
		log.Printf("application startup is incomplete: %v", a.initErr)
		return
	}
	cfg := a.store.get()
	runtime.WindowSetAlwaysOnTop(ctx, cfg.AlwaysOnTop)
	if cfg.ShortcutEnabled {
		a.startShortcut(cfg.DoubleTapMS)
	}
}

func (a *App) shutdown(context.Context) {
	log.Printf("application shutting down")
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shortcut != nil {
		a.shortcut.Stop()
		a.shortcut = nil
	}
	if a.local != nil {
		a.local.Close()
	}
}

func (a *App) closeLog() {
	if a.logFile == nil {
		return
	}
	log.Printf("application stopped")
	_ = a.logFile.Sync()
	log.SetOutput(os.Stderr)
	_ = a.logFile.Close()
	a.logFile = nil
}

func (a *App) GetConfig() (Config, error) {
	if a.initErr != nil {
		return Config{}, a.initErr
	}
	return a.store.get(), nil
}

func (a *App) SaveConfig(cfg Config) error {
	if a.initErr != nil {
		return a.initErr
	}
	previous := a.store.get()
	if err := a.store.save(cfg); err != nil {
		log.Printf("saving configuration failed: %v", err)
		return err
	}
	cfg = a.store.get()
	log.Printf("configuration saved; provider=%s local_model=%s shortcut=%t always_on_top=%t", cfg.Provider, cfg.LocalModel, cfg.ShortcutEnabled, cfg.AlwaysOnTop)
	if a.ctx != nil {
		runtime.WindowSetAlwaysOnTop(a.ctx, cfg.AlwaysOnTop)
	}
	if previous.LocalModel != cfg.LocalModel && a.local != nil {
		a.local.Close()
	}
	if previous.ShortcutEnabled != cfg.ShortcutEnabled || previous.DoubleTapMS != cfg.DoubleTapMS {
		a.stopShortcut()
		if cfg.ShortcutEnabled {
			a.startShortcut(cfg.DoubleTapMS)
		}
	}
	return nil
}

func (a *App) SetAlwaysOnTop(enabled bool) error {
	if a.initErr != nil {
		return a.initErr
	}
	cfg := a.store.get()
	cfg.AlwaysOnTop = enabled
	if err := a.store.save(cfg); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.WindowSetAlwaysOnTop(a.ctx, enabled)
	}
	return nil
}

func (a *App) Translate(req TranslateRequest) (TranslateResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return TranslateResult{}, errors.New("请输入要翻译的文本")
	}
	if len([]rune(text)) > 12000 {
		return TranslateResult{}, errors.New("单次翻译最多支持 12000 个字符")
	}
	if req.Source == req.Target {
		return TranslateResult{Text: text, Provider: "direct", Model: "原文"}, nil
	}
	cfg := a.store.get()
	log.Printf("translation started; provider=%s source=%s target=%s characters=%d", cfg.Provider, req.Source, req.Target, len([]rune(text)))
	if req.Source == "" {
		req.Source = cfg.SourceLanguage
	}
	if req.Target == "" {
		req.Target = cfg.TargetLanguage
	}
	if cfg.Provider == "llm" {
		translated, err := translateWithLLM(a.ctx, cfg, req)
		if err != nil {
			log.Printf("LLM translation failed; model=%s error=%v", cfg.LLMModel, err)
			return TranslateResult{}, err
		}
		log.Printf("LLM translation completed; model=%s", cfg.LLMModel)
		return TranslateResult{Text: translated, Provider: "llm", Model: cfg.LLMModel}, nil
	}
	if a.local == nil {
		return TranslateResult{}, errors.New("本地翻译服务未初始化")
	}
	if cfg.LocalModel == defaultLocalModel && (req.Source != "en" || req.Target != "zh") {
		return TranslateResult{}, errors.New("Helsinki 英中模型仅支持英语 → 中文；其他语言请选择 NLLB-200 或 LLM")
	}
	translated, err := a.local.Translate(cfg.LocalModel, req)
	if err != nil {
		log.Printf("local translation failed; model=%s error=%v", cfg.LocalModel, err)
		return TranslateResult{}, err
	}
	log.Printf("local translation completed; model=%s", cfg.LocalModel)
	return TranslateResult{Text: translated, Provider: "local", Model: cfg.LocalModel}, nil
}

func (a *App) ListModels() ([]ModelInfo, error) {
	if a.local == nil {
		return nil, errors.New("本地模型服务未初始化")
	}
	cfg := a.store.get()
	models := []ModelInfo{
		{
			ID: defaultLocalModel, Name: "OPUS-MT 英中", Size: "约 1.2 GB", Default: true,
			Description: "轻量、快速，专用于英语翻译成中文",
		},
		{
			ID: defaultNLLBModel, Name: "Meta NLLB-200", Size: "约 2.5 GB",
			Description: "支持 200 种语言，适合多语言互译",
		},
	}
	for i := range models {
		models[i].Downloaded = a.local.IsDownloaded(models[i].ID)
		models[i].Selected = cfg.LocalModel == models[i].ID
	}
	return models, nil
}

func (a *App) DownloadModel(modelID string) error {
	if modelID != defaultLocalModel && modelID != defaultNLLBModel {
		return errors.New("不支持的模型")
	}
	if a.local == nil {
		return errors.New("本地模型服务未初始化")
	}
	if err := a.beginModelDownload(modelID); err != nil {
		return err
	}
	log.Printf("model download started; model=%s", modelID)
	err := a.local.Download(modelID, func(progress modelDownloadProgress) {
		a.setModelDownloadStatus(ModelDownloadStatus{
			Model: modelID, Status: "downloading", Message: progress.Message, Progress: progress.Percent,
		})
	})
	if err != nil {
		progress := a.GetModelDownloadStatus().Progress
		a.setModelDownloadStatus(ModelDownloadStatus{Model: modelID, Status: "error", Message: err.Error(), Progress: progress})
		log.Printf("model download failed; model=%s error=%v", modelID, err)
		return err
	}
	a.setModelDownloadStatus(ModelDownloadStatus{Model: modelID, Status: "ready", Message: "模型已就绪", Progress: 100})
	log.Printf("model download completed; model=%s", modelID)
	return nil
}

func (a *App) GetModelDownloadStatus() ModelDownloadStatus {
	a.downloadMu.RLock()
	defer a.downloadMu.RUnlock()
	return a.downloadStatus
}

func (a *App) beginModelDownload(modelID string) error {
	a.downloadMu.Lock()
	if a.downloadStatus.Status == "downloading" {
		activeModel := a.downloadStatus.Model
		a.downloadMu.Unlock()
		return fmt.Errorf("模型 %s 正在下载，请等待当前下载完成", activeModel)
	}
	status := ModelDownloadStatus{
		Model: modelID, Status: "downloading", Message: "正在连接 Hugging Face…", Progress: -1,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	a.downloadStatus = status
	a.downloadMu.Unlock()
	a.emitModelDownloadStatus(status)
	return nil
}

func (a *App) setModelDownloadStatus(status ModelDownloadStatus) {
	status.UpdatedAt = time.Now().Format(time.RFC3339)
	a.downloadMu.Lock()
	a.downloadStatus = status
	a.downloadMu.Unlock()
	a.emitModelDownloadStatus(status)
}

func (a *App) emitModelDownloadStatus(status ModelDownloadStatus) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "model-download-status", status)
	}
}

func (a *App) PrepareRuntime() error {
	if a.local == nil {
		return errors.New("本地模型服务未初始化")
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "runtime-status", "正在安装本地翻译运行环境，这可能需要几分钟…")
	}
	log.Printf("runtime preparation started")
	err := a.local.PrepareRuntime(func(message string) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "runtime-status", message)
		}
	})
	if err == nil && a.ctx != nil {
		runtime.EventsEmit(a.ctx, "runtime-status", "运行环境已就绪")
	}
	if err != nil {
		log.Printf("runtime preparation failed: %v", err)
	} else {
		log.Printf("runtime preparation completed")
	}
	return err
}

func (a *App) GetRuntimeStatus() RuntimeStatus {
	if a.local == nil {
		return RuntimeStatus{Message: "服务初始化失败"}
	}
	return a.local.RuntimeStatus()
}

func (a *App) OpenDataDirectory() error {
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(base, "QuickTranslation")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if a.ctx == nil {
		return fmt.Errorf("窗口尚未就绪")
	}
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(path))
	return nil
}

func (a *App) startShortcut(doubleTapMS int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shortcut != nil {
		return
	}
	listener, err := newShortcutListener(doubleTapMS, func() {
		text, captureErr := captureSelectedText()
		if captureErr != nil || strings.TrimSpace(text) == "" || a.ctx == nil {
			return
		}
		runtime.WindowShow(a.ctx)
		runtime.WindowUnminimise(a.ctx)
		runtime.EventsEmit(a.ctx, "quick-translate", text)
	})
	if err != nil {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "shortcut-error", err.Error())
		}
		return
	}
	a.shortcut = listener
}

func (a *App) stopShortcut() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shortcut != nil {
		a.shortcut.Stop()
		a.shortcut = nil
	}
}
