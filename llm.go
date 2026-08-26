package main

import (
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
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(respBody, &apiErr)
		message := strings.TrimSpace(apiErr.Error.Message)
		if message == "" {
			message = strings.TrimSpace(string(respBody))
		}
		if len(message) > 500 {
			message = message[:500]
		}
		return "", fmt.Errorf("LLM 返回错误 (%d): %s", resp.StatusCode, message)
	}
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
