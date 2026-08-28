package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var languageNames = map[string]string{
	"en": "英语", "zh": "简体中文", "ja": "日语", "ko": "韩语",
	"fr": "法语", "de": "德语", "es": "西班牙语", "ru": "俄语",
}

func translateWithLLM(ctx context.Context, cfg Config, req TranslateRequest) (string, error) {
	return translateWithLLMStream(ctx, cfg, req, nil)
}

func translateWithLLMStream(ctx context.Context, cfg Config, req TranslateRequest, onDelta func(string)) (string, error) {
	if strings.TrimSpace(cfg.LLMBaseURL) == "" {
		return "", errors.New("请先配置 LLM Base URL")
	}
	if strings.TrimSpace(cfg.LLMModel) == "" {
		return "", errors.New("请先配置 LLM 模型名称")
	}
	base := strings.TrimRight(cfg.LLMBaseURL, "/")
	endpoint := base
	if !strings.HasSuffix(strings.ToLower(endpoint), "/chat/completions") {
		if !strings.HasSuffix(strings.ToLower(endpoint), "/v1") {
			endpoint += "/v1"
		}
		endpoint += "/chat/completions"
	}
	source := languageNames[req.Source]
	target := languageNames[req.Target]
	if source == "" {
		source = req.Source
	}
	if target == "" {
		target = req.Target
	}
	payload := map[string]any{
		"model": cfg.LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是专业翻译引擎。只输出译文，不解释、不加引号，并保持原文的段落、格式、语气和专有名词。"},
			{"role": "user", "content": fmt.Sprintf("将以下%s文本翻译为%s：\n\n%s", source, target, req.Text)},
		},
		"temperature": 0,
		"stream":      true,
	}
	body, _ := json.Marshal(payload)
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.LLMAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.LLMAPIKey)
	}
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", readLLMErrorResponse(resp)
	}

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return readLLMStream(resp.Body, onDelta)
	}

	// A few OpenAI-compatible services ignore stream=true and return a normal
	// JSON completion. Keep supporting those services as a transparent fallback.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	translated, err := decodeLLMCompletion(respBody)
	if err != nil {
		return "", err
	}
	if onDelta != nil {
		onDelta(translated)
	}
	return translated, nil
}

func readLLMErrorResponse(resp *http.Response) error {
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("LLM 返回错误 (%d)，且无法读取响应: %w", resp.StatusCode, err)
	}
	var result struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(respBody, &result)
	message := strings.TrimSpace(result.Error.Message)
	if message == "" {
		message = strings.TrimSpace(string(respBody))
	}
	if len(message) > 500 {
		message = message[:500]
	}
	return fmt.Errorf("LLM 返回错误 (%d): %s", resp.StatusCode, message)
}

func decodeLLMCompletion(respBody []byte) (string, error) {
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("无法解析 LLM 响应: %w", err)
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", errors.New("LLM 没有返回译文")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func readLLMStream(reader io.Reader, onDelta func(string)) (string, error) {
	scanner := bufio.NewScanner(reader)
	// Individual SSE records are normally tiny, but allow larger records for
	// gateways that batch many tokens into one event.
	scanner.Buffer(make([]byte, 64<<10), 4<<20)

	var translated strings.Builder
	var dataLines []string
	done := false

	flushEvent := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if data == "" {
			return nil
		}
		if data == "[DONE]" {
			done = true
			return nil
		}

		var event struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("无法解析 LLM 流式响应: %w", err)
		}
		if event.Error != nil {
			message := strings.TrimSpace(event.Error.Message)
			if message == "" {
				message = "未知错误"
			}
			return fmt.Errorf("LLM 流式响应错误: %s", message)
		}
		if len(event.Choices) == 0 {
			return nil
		}
		delta := event.Choices[0].Delta.Content
		if delta == "" {
			delta = event.Choices[0].Message.Content
		}
		if delta != "" {
			translated.WriteString(delta)
			if onDelta != nil {
				onDelta(delta)
			}
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flushEvent(); err != nil {
				return "", err
			}
			if done {
				break
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found || field != "data" {
			continue
		}
		dataLines = append(dataLines, strings.TrimPrefix(value, " "))
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取 LLM 流式响应失败: %w", err)
	}
	if !done {
		if err := flushEvent(); err != nil {
			return "", err
		}
	}
	result := strings.TrimSpace(translated.String())
	if result == "" {
		return "", errors.New("LLM 没有返回译文")
	}
	return result, nil
}
