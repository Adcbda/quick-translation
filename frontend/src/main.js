const languages = [
  ["en", "英语", "EN"],
  ["zh", "简体中文", "中"],
  ["ja", "日语", "日"],
  ["ko", "韩语", "한"],
  ["fr", "法语", "FR"],
  ["de", "德语", "DE"],
  ["es", "西班牙语", "ES"],
  ["ru", "俄语", "РУ"],
];

const windowMinimums = {
  default: [880, 600],
  minimal: [420, 320],
};

const state = {
  config: null,
  models: [],
  translating: false,
  activeTranslationID: "",
  lastTranslationSignature: "",
  pendingBlurTranslation: null,
  queuedClipboardText: "",
  result: "",
  modelDownload: { model: "", status: "idle", message: "", progress: -1 },
  capturingShortcut: false,
  shortcutCaptureModifier: "",
  shortcutCaptureAt: 0,
};

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => [...document.querySelectorAll(selector)];

function backend() {
  return window.go?.main?.App;
}

function toast(message, type = "info", duration = 3200) {
  const element = document.createElement("div");
  element.className = `toast ${type}`;
  element.innerHTML = `<span>${type === "error" ? "!" : type === "success" ? "✓" : "i"}</span><p>${escapeHTML(message)}</p>`;
  $("#toastContainer").appendChild(element);
  requestAnimationFrame(() => element.classList.add("show"));
  setTimeout(() => {
    element.classList.remove("show");
    setTimeout(() => element.remove(), 220);
  }, duration);
}

function errorMessage(error) {
  if (!error) return "发生未知错误";
  if (typeof error === "string") return error.replace(/^Error:\s*/i, "");
  return String(error.message || error).replace(/^Error:\s*/i, "");
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function initialiseLanguages() {
  for (const selector of ["#sourceLanguage", "#targetLanguage"]) {
    const select = $(selector);
    select.innerHTML = languages.map(([code, name]) => `<option value="${code}">${name}</option>`).join("");
  }
}

async function loadApp() {
  initialiseLanguages();
  wireInteractions();
  wireRuntimeEvents();
  const api = backend();
  if (!api) {
    toast("请通过 Wails 启动桌面应用", "error", 6000);
    return;
  }
  try {
    state.config = await api.GetConfig();
    applyConfig();
    await Promise.all([refreshModels(), refreshRuntimeStatus(), refreshModelDownloadStatus()]);
  } catch (error) {
    toast(errorMessage(error), "error", 6000);
  }
}

function applyConfig() {
  const cfg = state.config;
  $("#sourceLanguage").value = cfg.sourceLanguage || "en";
  $("#targetLanguage").value = cfg.targetLanguage || "zh";
  $("#llmBaseUrl").value = cfg.llmBaseUrl || "";
  $("#llmModel").value = cfg.llmModel || "";
  $("#llmApiKey").value = cfg.llmApiKey || "";
  $("#clipboardEnabled").checked = Boolean(cfg.clipboardEnabled);
  $("#quickOpenEnabled").checked = Boolean(cfg.quickOpenEnabled);
  $("#closeToTray").checked = Boolean(cfg.closeToTray);
  $("#topmostButton").classList.toggle("active", Boolean(cfg.alwaysOnTop));
  applyTheme();
  applyMinimalMode();
  $$("#providerControl button").forEach((button) => button.classList.toggle("active", button.dataset.provider === cfg.provider));
  $("#llmSettings").classList.toggle("muted", cfg.provider !== "llm");
  updateEngineBadge();
  renderShortcutControls();
  updateShortcutHint();
}

function updateEngineBadge() {
  if (!state.config) return;
  const badge = $("#engineBadge");
  if (state.config.provider === "llm") {
    badge.className = "engine-badge llm";
    badge.querySelector("strong").textContent = state.config.llmModel || "LLM 接口";
  } else {
    badge.className = "engine-badge";
    badge.querySelector("strong").textContent = state.config.localModel?.includes("nllb") ? "NLLB-200" : "OPUS-MT 本地";
  }
}

function updateShortcutHint() {
  const enabled = Boolean(state.config?.quickOpenEnabled);
  $(".shortcut-hint").classList.toggle("disabled", !enabled);
  $(".pulse-dot").classList.toggle("off", !enabled);
  $("#quickOpenHint").textContent = enabled ? `${shortcutLabel(state.config?.quickOpenShortcut)} 快速唤起` : "快捷键唤起已关闭";
}

function shortcutLabel(shortcut) {
  const value = String(shortcut || "DoubleCtrl");
  if (value.startsWith("Double")) return `双击 ${value.slice(6)}`;
  return value.split("+").join(" + ");
}

function renderShortcutControls() {
  const shortcut = state.config?.quickOpenShortcut || "DoubleCtrl";
  const enabled = $("#quickOpenEnabled").checked;
  $("#shortcutDisplay").textContent = shortcutLabel(shortcut);
  $("#shortcutConfig").classList.toggle("disabled", !enabled);
  $("#shortcutRecorder").disabled = !enabled;
  $("#resetShortcut").disabled = !enabled || shortcut === "DoubleCtrl";
  if (!state.capturingShortcut) {
    $("#shortcutRecorder em").textContent = "点击修改";
  }
}

function wireInteractions() {
  $$(".nav-item").forEach((button) => button.addEventListener("click", () => showView(button.dataset.view)));
  $("#sourceText").addEventListener("input", updateCharacterCount);
  $("#sourceText").addEventListener("blur", translateStableInput);
  $("#sourceText").addEventListener("keydown", (event) => {
    if (event.key === "Enter" && event.ctrlKey) {
      event.preventDefault();
      translate();
    }
  });
  $("#translateButton").addEventListener("click", translate);
  $("#clearInput").addEventListener("click", clearTranslation);
  $("#copyResult").addEventListener("click", copyResult);
  $("#swapLanguages").addEventListener("click", swapLanguages);
  $("#sourceLanguage").addEventListener("change", rememberLanguages);
  $("#targetLanguage").addEventListener("change", rememberLanguages);
  $("#minimalMode").addEventListener("change", toggleMinimalMode);
  $("#darkMode").addEventListener("change", previewDarkMode);
  $("#quickOpenEnabled").addEventListener("change", previewQuickOpenEnabled);
  $("#closeToTray").addEventListener("change", previewCloseToTray);
  $("#shortcutRecorder").addEventListener("click", startShortcutCapture);
  $("#resetShortcut").addEventListener("click", resetQuickOpenShortcut);
  document.addEventListener("keydown", captureShortcutKey, true);
  $("#topmostButton").addEventListener("click", toggleTopmost);
  $("#saveSettings").addEventListener("click", saveSettings);
  $("#prepareRuntime").addEventListener("click", prepareRuntime);
  $("#openDataDirectory").addEventListener("click", () => backend()?.OpenDataDirectory().catch((e) => toast(errorMessage(e), "error")));
  $("#toggleApiKey").addEventListener("click", () => {
    const input = $("#llmApiKey");
    input.type = input.type === "password" ? "text" : "password";
  });
  $$("#providerControl button").forEach((button) => button.addEventListener("click", () => selectProvider(button.dataset.provider)));
}

function previewQuickOpenEnabled(event) {
  if (state.capturingShortcut) finishShortcutCapture();
  renderShortcutControls();
  $("#saveHint").textContent = event.currentTarget.checked ? "快捷键唤起已开启，请保存设置" : "快捷键唤起已关闭，请保存设置";
}

function previewCloseToTray(event) {
  $("#saveHint").textContent = event.currentTarget.checked ? "关闭到托盘已开启，请保存设置" : "关闭时退出程序，请保存设置";
}

function startShortcutCapture() {
  if (state.capturingShortcut) {
    finishShortcutCapture();
    return;
  }
  state.capturingShortcut = true;
  state.shortcutCaptureModifier = "";
  state.shortcutCaptureAt = 0;
  $("#shortcutRecorder").classList.add("recording");
  $("#shortcutRecorder em").textContent = "录入中";
  $("#shortcutDisplay").textContent = "请按快捷键…";
  $("#shortcutHelp").textContent = "可双击 Ctrl / Shift / Alt，或按下带修饰键的组合；Esc 取消";
}

function finishShortcutCapture() {
  state.capturingShortcut = false;
  state.shortcutCaptureModifier = "";
  state.shortcutCaptureAt = 0;
  $("#shortcutRecorder").classList.remove("recording");
  $("#shortcutHelp").textContent = "点击右侧按键，然后按下新的快捷键";
  renderShortcutControls();
}

function captureShortcutKey(event) {
  if (!state.capturingShortcut) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  if (event.repeat) return;
  if (event.key === "Escape") {
    finishShortcutCapture();
    return;
  }
  if (event.metaKey || event.key === "Meta") {
    $("#shortcutHelp").textContent = "暂不支持 Windows 键，请选择 Ctrl、Shift 或 Alt";
    return;
  }

  const modifier = { Control: "Ctrl", Shift: "Shift", Alt: "Alt" }[event.key];
  if (modifier) {
    const now = performance.now();
    if (state.shortcutCaptureModifier === modifier && now - state.shortcutCaptureAt <= 700) {
      acceptCapturedShortcut(`Double${modifier}`);
      return;
    }
    state.shortcutCaptureModifier = modifier;
    state.shortcutCaptureAt = now;
    $("#shortcutDisplay").textContent = `再按一次 ${modifier}`;
    $("#shortcutHelp").textContent = `再次按下 ${modifier} 可设为双击唤起，也可继续按一个普通按键`;
    return;
  }

  const key = shortcutKeyFromEvent(event);
  if (!key) {
    $("#shortcutHelp").textContent = "这个按键暂不支持，请换一个字母、数字、F1–F12 或常用功能键";
    return;
  }
  const modifiers = [];
  if (event.ctrlKey) modifiers.push("Ctrl");
  if (event.shiftKey) modifiers.push("Shift");
  if (event.altKey) modifiers.push("Alt");
  if (!modifiers.length) {
    $("#shortcutHelp").textContent = "组合快捷键至少需要 Ctrl、Shift 或 Alt 中的一个";
    return;
  }
  acceptCapturedShortcut([...modifiers, key].join("+"));
}

function shortcutKeyFromEvent(event) {
  if (/^Key[A-Z]$/.test(event.code)) return event.code.slice(3);
  if (/^Digit[0-9]$/.test(event.code)) return event.code.slice(5);
  if (/^F(?:[1-9]|1[0-2])$/.test(event.key)) return event.key;
  return {
    Space: "Space",
    Enter: "Enter",
    Tab: "Tab",
    Backspace: "Backspace",
    Delete: "Delete",
    Insert: "Insert",
    Home: "Home",
    End: "End",
    PageUp: "PageUp",
    PageDown: "PageDown",
    ArrowLeft: "Left",
    ArrowUp: "Up",
    ArrowRight: "Right",
    ArrowDown: "Down",
  }[event.key] || "";
}

function acceptCapturedShortcut(shortcut) {
  state.config.quickOpenShortcut = shortcut;
  finishShortcutCapture();
  $("#shortcutHelp").textContent = "快捷键已修改，保存设置后生效";
  $("#saveHint").textContent = "唤起快捷键已修改，请保存设置";
}

function resetQuickOpenShortcut() {
  if (!state.config) return;
  state.config.quickOpenShortcut = "DoubleCtrl";
  finishShortcutCapture();
  $("#shortcutHelp").textContent = "已恢复为双击 Ctrl，保存设置后生效";
  $("#saveHint").textContent = "唤起快捷键已恢复默认，请保存设置";
}

function setMinimalModeAppearance(enabled) {
  $("#app").classList.toggle("minimal-mode", enabled);
  document.body.classList.toggle("minimal-mode", enabled);
  $("#minimalMode").checked = enabled;
  const [width, height] = enabled ? windowMinimums.minimal : windowMinimums.default;
  window.runtime?.WindowSetMinSize?.(width, height);
}

function applyMinimalMode() {
  setMinimalModeAppearance(Boolean(state.config?.minimalMode));
}

function setThemeAppearance(enabled) {
  const theme = enabled ? "dark" : "light";
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
  $("#darkMode").checked = enabled;
  $("#themeColor").content = enabled ? "#11131b" : "#f5f6fa";
}

function applyTheme() {
  setThemeAppearance(Boolean(state.config?.darkMode));
}

function previewDarkMode(event) {
  const enabled = event.currentTarget.checked;
  if (state.config) state.config.darkMode = enabled;
  setThemeAppearance(enabled);
  $("#saveHint").textContent = "外观已修改，请保存设置";
}

async function toggleMinimalMode(event) {
  if (!state.config) {
    setMinimalModeAppearance(event.currentTarget.checked);
    return;
  }
  const previous = Boolean(state.config.minimalMode);
  state.config.minimalMode = event.currentTarget.checked;
  applyMinimalMode();
  try {
    await backend().SaveConfig(state.config);
  } catch (error) {
    state.config.minimalMode = previous;
    applyMinimalMode();
    toast(errorMessage(error), "error");
  }
}

function wireRuntimeEvents() {
  const eventsOn = window.runtime?.EventsOn;
  if (!eventsOn) return;
  eventsOn("clipboard-translate", translateClipboardText);
  eventsOn("clipboard-error", (message) => toast(message, "error", 6000));
  eventsOn("quick-open", () => {
    showView("translate");
    requestAnimationFrame(() => $("#sourceText").focus());
  });
  eventsOn("quick-open-error", (message) => toast(message, "error", 6000));
  eventsOn("runtime-status", (message) => {
    $("#runtimeMessage").textContent = message;
  });
  eventsOn("model-download-status", (payload) => {
    updateModelDownloadStatus(payload);
  });
  eventsOn("translation-stream", appendTranslationChunk);
}

function appendTranslationChunk(payload) {
  if (!state.translating || payload?.requestId !== state.activeTranslationID) return;
  const delta = String(payload?.delta || "");
  if (!delta) return;
  state.result += delta;
  const resultNode = $("#resultText");
  resultNode.className = "result-content streaming-result";
  resultNode.textContent = state.result;
  resultNode.scrollTop = resultNode.scrollHeight;
  $("#resultMeta").textContent = "LLM · 正在接收译文…";
  $("#translationStatus").textContent = "正在流式翻译…";
}

function translateClipboardText(value) {
  const text = String(value || "").trim();
  if (!text) return;
  showView("translate");
  if (state.translating) {
    state.queuedClipboardText = text;
    $("#translationStatus").textContent = "已收到新的剪贴板文本，将在当前翻译后继续…";
    return;
  }
  state.queuedClipboardText = "";
  $("#sourceText").value = text;
  updateCharacterCount();
  translate();
}

async function refreshModelDownloadStatus() {
  try {
    updateModelDownloadStatus(await backend().GetModelDownloadStatus());
  } catch (error) {
    console.warn("无法读取模型下载状态", error);
  }
}

function updateModelDownloadStatus(payload) {
  state.modelDownload = {
    model: payload?.model || "",
    status: payload?.status || "idle",
    message: payload?.message || "",
    progress: Number.isFinite(Number(payload?.progress)) ? Number(payload.progress) : -1,
    updatedAt: payload?.updatedAt || "",
  };
  renderGlobalDownloadStatus();
  applyDownloadStatusToModelCards();
}

function downloadModelName(modelID) {
  return state.models.find((model) => model.id === modelID)?.name || modelID || "本地模型";
}

function renderGlobalDownloadStatus() {
  const status = state.modelDownload;
  const container = $("#globalDownloadStatus");
  const downloading = status.status === "downloading";
  container.hidden = !downloading;
  if (!downloading) return;

  const hasPercent = status.progress >= 0 && status.progress <= 100;
  $("#globalDownloadTitle").textContent = `正在下载 ${downloadModelName(status.model)}`;
  $("#globalDownloadPercent").textContent = hasPercent ? `当前文件 ${status.progress}%` : "正在连接";
  $("#globalDownloadMessage").textContent = status.message || "正在从 Hugging Face 下载模型…";
  $("#globalProgressBar").style.width = hasPercent ? `${status.progress}%` : "";
  $("#globalProgressTrack").classList.toggle("indeterminate", !hasPercent);
}

function applyDownloadStatusToModelCards() {
  const status = state.modelDownload;
  $$(".model-card").forEach((card) => {
    const active = status.status === "downloading" && card.dataset.modelId === status.model;
    const message = card.querySelector(".download-message");
    const progressTrack = card.querySelector(".model-progress");
    const progressBar = progressTrack?.querySelector("i");
    const button = card.querySelector(".download-model");
    if (card.dataset.modelId === status.model && message) {
      message.textContent = status.message;
    }
    if (progressTrack) {
      const hasPercent = active && status.progress >= 0 && status.progress <= 100;
      progressTrack.hidden = !active;
      progressTrack.classList.toggle("indeterminate", active && !hasPercent);
      if (progressBar) progressBar.style.width = hasPercent ? `${status.progress}%` : "";
    }
    if (button) {
      button.disabled = status.status === "downloading";
      if (active) {
        button.innerHTML = `<i class="mini-spinner"></i>下载中`;
      } else if (status.status !== "downloading") {
        button.innerHTML = `<svg viewBox="0 0 24 24"><path d="M12 3v12M7 10l5 5 5-5M4 21h16"/></svg>下载模型`;
      }
    }
  });
}

function showView(name) {
  const titles = {
    translate: ["TRANSLATOR", "让语言不再成为边界"],
    models: ["LOCAL MODELS", "模型管理"],
    settings: ["PREFERENCES", "设置"],
  };
  $$(".nav-item").forEach((button) => button.classList.toggle("active", button.dataset.view === name));
  $$(".view").forEach((view) => view.classList.remove("active"));
  $(`#${name}View`).classList.add("active");
  $("#pageEyebrow").textContent = titles[name][0];
  $("#pageTitle").textContent = titles[name][1];
  if (name === "models") {
    refreshModels();
    refreshRuntimeStatus();
  }
}

async function rememberLanguages() {
  if (!state.config) return;
  state.config.sourceLanguage = $("#sourceLanguage").value;
  state.config.targetLanguage = $("#targetLanguage").value;
  try {
    await backend().SaveConfig(state.config);
  } catch (error) {
    toast(errorMessage(error), "error");
  }
}

async function swapLanguages() {
  const source = $("#sourceLanguage");
  const target = $("#targetLanguage");
  [source.value, target.value] = [target.value, source.value];
  const result = state.result;
  if (result) {
    $("#sourceText").value = result;
    clearResult();
    updateCharacterCount();
  }
  await rememberLanguages();
}

function updateCharacterCount() {
  $("#characterCount").textContent = `${[...$("#sourceText").value].length.toLocaleString()} / 12,000`;
}

function translationSignature(text) {
  return [$("#sourceLanguage").value, $("#targetLanguage").value, text].join("\u0000");
}

function translateStableInput() {
  const text = $("#sourceText").value.trim();
  if (!text) return;

  const candidate = { text, signature: translationSignature(text) };
  setTimeout(() => {
    const currentText = $("#sourceText").value.trim();
    if (!currentText || currentText !== candidate.text || translationSignature(currentText) !== candidate.signature) return;
    if (state.lastTranslationSignature === candidate.signature) return;
    if (state.translating) {
      state.pendingBlurTranslation = candidate;
      return;
    }
    state.pendingBlurTranslation = null;
    translate();
  }, 0);
}

function clearResult() {
  state.result = "";
  const result = $("#resultText");
  result.className = "result-content empty";
  result.innerHTML = `<div class="result-placeholder"><span class="placeholder-icon"><svg viewBox="0 0 24 24"><path d="M4 5h16v12H8l-4 4V5Z"/><path d="M8 9h8M8 13h5"/></svg></span><strong>译文将在这里显示</strong><small>由你选择的本地模型或 LLM 提供翻译</small></div>`;
  $("#copyResult").disabled = true;
  $("#resultMeta").textContent = "准备就绪";
}

function clearTranslation() {
  $("#sourceText").value = "";
  updateCharacterCount();
  clearResult();
  $("#sourceText").focus();
}

async function translate() {
  const text = $("#sourceText").value.trim();
  if (!text || state.translating) {
    if (!text) toast("请先输入要翻译的文本", "error");
    return;
  }
  state.translating = true;
  state.lastTranslationSignature = translationSignature(text);
  state.activeTranslationID = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`;
  state.result = "";
  $("#translateButton").classList.add("loading");
  $("#translateButton").disabled = true;
  $("#translationStatus").textContent = "正在调用翻译引擎…";
  const resultNode = $("#resultText");
  const streaming = state.config?.provider === "llm";
  if (streaming) {
    resultNode.className = "result-content streaming-result";
    resultNode.textContent = "";
    $("#resultMeta").textContent = "LLM · 正在等待响应…";
  } else {
    resultNode.className = "result-content loading-result";
    resultNode.innerHTML = `<div class="skeleton-lines"><i></i><i></i><i></i><i></i></div>`;
  }
  const started = performance.now();
  try {
    const result = await backend().Translate({
      text,
      source: $("#sourceLanguage").value,
      target: $("#targetLanguage").value,
      requestId: state.activeTranslationID,
    });
    state.result = result.text;
    resultNode.className = "result-content";
    resultNode.textContent = result.text;
    $("#copyResult").disabled = false;
    const elapsed = ((performance.now() - started) / 1000).toFixed(1);
    $("#resultMeta").textContent = `${result.provider === "local" ? "本地" : result.provider === "llm" ? "LLM" : "原文"} · ${elapsed}s`;
    $("#translationStatus").textContent = `由 ${result.model} 完成`;
  } catch (error) {
    clearResult();
    const message = errorMessage(error);
    $("#translationStatus").textContent = message;
    toast(message, "error", 6000);
  } finally {
    state.translating = false;
    state.activeTranslationID = "";
    $("#translateButton").classList.remove("loading");
    $("#translateButton").disabled = false;
    if (state.queuedClipboardText) {
      const nextText = state.queuedClipboardText;
      state.queuedClipboardText = "";
      setTimeout(() => translateClipboardText(nextText), 0);
    } else if (state.pendingBlurTranslation) {
      const candidate = state.pendingBlurTranslation;
      state.pendingBlurTranslation = null;
      setTimeout(() => {
        const currentText = $("#sourceText").value.trim();
        if (currentText === candidate.text && translationSignature(currentText) === candidate.signature) {
          translateStableInput();
        }
      }, 0);
    }
  }
}

async function copyResult() {
  if (!state.result) return;
  try {
    await backend().CopyText(state.result);
    const label = $("#copyResult span");
    label.textContent = "已复制";
    setTimeout(() => label.textContent = "复制", 1400);
  } catch {
    toast("复制失败，请手动选择译文", "error");
  }
}

async function toggleTopmost() {
  if (!state.config) return;
  const enabled = !state.config.alwaysOnTop;
  try {
    await backend().SetAlwaysOnTop(enabled);
    state.config.alwaysOnTop = enabled;
    $("#topmostButton").classList.toggle("active", enabled);
    toast(enabled ? "翻译窗口已置顶" : "已取消窗口置顶", "success");
  } catch (error) {
    toast(errorMessage(error), "error");
  }
}

async function refreshRuntimeStatus() {
  try {
    const status = await backend().GetRuntimeStatus();
    const card = $("#runtimeCard");
    card.classList.toggle("ready", status.ready);
    card.classList.toggle("error", !status.pythonFound);
    $("#runtimeTitle").textContent = status.ready ? "本地运行环境已就绪" : status.pythonFound ? "还需要安装模型依赖" : "未找到 Python 3";
    $("#runtimeMessage").textContent = status.message + (status.python ? ` · ${status.python}` : "");
    $("#prepareRuntime").style.display = status.ready ? "none" : "inline-flex";
  } catch (error) {
    $("#runtimeMessage").textContent = errorMessage(error);
  }
}

async function prepareRuntime() {
  const button = $("#prepareRuntime");
  button.disabled = true;
  button.textContent = "正在准备…";
  try {
    await backend().PrepareRuntime();
    toast("本地翻译运行环境已安装", "success");
    await refreshRuntimeStatus();
  } catch (error) {
    toast(errorMessage(error), "error", 8000);
  } finally {
    button.disabled = false;
    button.textContent = "准备运行环境";
  }
}

async function refreshModels() {
  try {
    state.models = await backend().ListModels();
    renderModels();
  } catch (error) {
    toast(errorMessage(error), "error");
  }
}

function renderModels() {
  $("#modelList").innerHTML = state.models.map((model) => {
    const activeDownload = state.modelDownload.status === "downloading" && state.modelDownload.model === model.id;
    const downloadDisabled = state.modelDownload.status === "downloading";
    return `
    <article class="model-card ${model.selected ? "selected" : ""}" data-model-id="${escapeHTML(model.id)}">
      <div class="model-symbol ${model.id.includes("nllb") ? "meta" : "opus"}">${model.id.includes("nllb") ? "M" : "文"}</div>
      <div class="model-copy">
        <div class="model-title"><h3>${escapeHTML(model.name)}</h3>${model.default ? '<span class="default-pill">默认</span>' : ""}${model.selected ? '<span class="selected-pill">使用中</span>' : ""}</div>
        <p>${escapeHTML(model.description)}</p>
        <div class="model-details"><span>${escapeHTML(model.id)}</span><i></i><span>${escapeHTML(model.size)}</span><i></i><span class="${model.downloaded ? "downloaded" : ""}">${model.downloaded ? "已下载" : "未下载"}</span></div>
        <small class="download-message"></small>
        <span class="model-progress" hidden><i></i></span>
      </div>
      <div class="model-actions">
        ${!model.downloaded ? `<button class="secondary-button download-model" data-id="${escapeHTML(model.id)}" ${downloadDisabled ? "disabled" : ""}>${activeDownload ? '<i class="mini-spinner"></i>下载中' : '<svg viewBox="0 0 24 24"><path d="M12 3v12M7 10l5 5 5-5M4 21h16"/></svg>下载模型'}</button>` : ""}
        ${model.downloaded && !model.selected ? `<button class="secondary-button select-model" data-id="${escapeHTML(model.id)}">切换使用</button>` : ""}
        ${model.selected ? '<span class="active-check"><svg viewBox="0 0 24 24"><path d="m6 12 4 4 8-8"/></svg></span>' : ""}
      </div>
    </article>`;
  }).join("");
  $$(".download-model").forEach((button) => button.addEventListener("click", () => downloadModel(button.dataset.id, button)));
  $$(".select-model").forEach((button) => button.addEventListener("click", () => selectModel(button.dataset.id)));
  renderGlobalDownloadStatus();
  applyDownloadStatusToModelCards();
}

async function downloadModel(modelID, button) {
  button.disabled = true;
  button.innerHTML = `<i class="mini-spinner"></i>下载中`;
  if (state.modelDownload.status !== "downloading") {
    updateModelDownloadStatus({ model: modelID, status: "downloading", message: "正在连接 Hugging Face…", progress: -1 });
  }
  try {
    await backend().DownloadModel(modelID);
    toast("模型下载完成", "success");
    await refreshModels();
  } catch (error) {
    toast(errorMessage(error), "error", 8000);
    await refreshModels();
  }
}

async function selectModel(modelID) {
  state.config.localModel = modelID;
  state.config.provider = "local";
  try {
    await backend().SaveConfig(state.config);
    applyConfig();
    await refreshModels();
    toast("已切换本地模型", "success");
  } catch (error) {
    toast(errorMessage(error), "error");
  }
}

function selectProvider(provider) {
  state.config.provider = provider;
  $$("#providerControl button").forEach((button) => button.classList.toggle("active", button.dataset.provider === provider));
  $("#llmSettings").classList.toggle("muted", provider !== "llm");
  $("#saveHint").textContent = "翻译引擎已修改，请保存设置";
}

async function saveSettings() {
  if (!state.config) return;
  state.config.llmBaseUrl = $("#llmBaseUrl").value.trim();
  state.config.llmModel = $("#llmModel").value.trim();
  state.config.llmApiKey = $("#llmApiKey").value.trim();
  state.config.clipboardEnabled = $("#clipboardEnabled").checked;
  state.config.quickOpenEnabled = $("#quickOpenEnabled").checked;
  state.config.closeToTray = $("#closeToTray").checked;
  state.config.darkMode = $("#darkMode").checked;
  state.config.sourceLanguage = $("#sourceLanguage").value;
  state.config.targetLanguage = $("#targetLanguage").value;
  try {
    await backend().SaveConfig(state.config);
    applyConfig();
    finishShortcutCapture();
    $("#shortcutHelp").textContent = "当前快捷键已生效；点击右侧按键可再次修改";
    $("#saveHint").textContent = "所有设置已保存";
    toast("设置已保存", "success");
  } catch (error) {
    try {
      state.config = await backend().GetConfig();
      applyConfig();
      finishShortcutCapture();
    } catch {
      // Keep the original save error as the actionable message.
    }
    toast(errorMessage(error), "error", 6000);
  }
}

loadApp();
