# Quick Translation

一个使用 Go + Wails 构建的本地优先桌面翻译工具。默认使用 `Helsinki-NLP/opus-mt-en-zh` 将英语翻译为中文，也可切换到 Meta NLLB-200 或任意 OpenAI 兼容 LLM 接口。

## 功能

- 英语 → 中文默认翻译，可在界面中切换 8 种常用语言
- 本地模型下载、跨页面实时进度、状态检查与切换
- OpenAI 兼容接口配置：Base URL、Model、API Key，支持流式显示译文
- 全局快捷键快速唤起窗口（默认双击 `Ctrl`，可在设置中录入其他快捷键）
- 复制文本后监听剪贴板变化并自动翻译（Windows）
- 翻译窗口置顶、结果一键复制
- 配置保存在本地用户配置目录

## 技术结构

- 桌面外壳与业务逻辑：Go 1.23 + Wails v2
- 界面：原生 HTML/CSS/JavaScript + Vite
- 本地推理：Python 工作进程 + Transformers/PyTorch
- 本地模型缓存：`%AppData%/QuickTranslation/models`（Windows）
- 程序日志：`%AppData%/QuickTranslation/logs/quick-translation.log`（Windows）

本地模型由 Go 应用管理，但 PyTorch/Transformers 的推理由随项目提供的 Python 工作进程执行。模型加载后进程会常驻，后续翻译不需要反复加载。

## 开发运行

要求：

- Go 1.23+
- Node.js 18+
- Wails CLI v2
- Microsoft WebView2 Runtime（Windows 10/11 通常已包含）
- Python 3.10+（使用本地模型时；是否有 PyTorch 预编译包取决于 Python 版本与平台）

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
wails dev
```

第一次使用本地模型：

1. 打开“模型管理”。
2. 点击“准备运行环境”，安装 PyTorch、Transformers 与 SentencePiece。
3. 下载需要的模型。
4. 下载完成后选择模型并开始翻译。

准备环境和下载模型都会访问网络，并会占用数 GB 磁盘空间。模型下载完成后可离线翻译。

## 构建

```bash
wails build -platform windows/amd64
```

构建产物位于 `build/bin/QuickTranslation.exe`。

## OpenAI 兼容接口

在“设置”中切换到“LLM 接口”，填写：

- Base URL：例如 `https://api.openai.com/v1`，也可填写仅到域名的网关地址
- 模型名称：接口支持的模型 ID
- API Key：可留空以连接不验证密钥的本地服务

应用自动补全 `/v1/chat/completions`，若 Base URL 已经以 `/chat/completions` 结尾则直接使用。LLM 请求默认启用 OpenAI 兼容的 SSE 流式响应；若服务端忽略流式参数并返回普通 JSON，应用也会自动兼容。API Key 当前保存在本地配置文件中，请注意操作系统账户安全。

## 剪贴板自动翻译说明

Windows 版本会在开启“剪贴板自动翻译”后监听剪贴板变化。在任意应用中复制非空文本，Quick Translation 会显示窗口并自动开始翻译。程序启动时已有的剪贴板内容不会触发；应用内“复制译文”也不会重复触发翻译。

## 全局快捷键

Windows 版本默认可在任意应用中双击 `Ctrl` 快速显示并聚焦翻译窗口。在“设置 → 快捷键快速唤起”中可关闭该功能，或录入双击 `Ctrl` / `Shift` / `Alt` 以及带修饰键的组合快捷键。新快捷键在保存设置后生效。
