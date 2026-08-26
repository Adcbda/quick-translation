package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranslateWithLLM(t *testing.T) {
	var receivedPath string
	var receivedAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuthorization = r.Header.Get("Authorization")
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好，世界！"}}]}`))
	}))
	defer server.Close()

	got, err := translateWithLLM(context.Background(), Config{
		LLMBaseURL: server.URL,
		LLMModel:   "test-model",
		LLMAPIKey:  "secret",
	}, TranslateRequest{Text: "Hello, world!", Source: "en", Target: "zh"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "你好，世界！" {
		t.Fatalf("translation = %q", got)
	}
	if receivedPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", receivedPath)
	}
	if receivedAuthorization != "Bearer secret" {
		t.Fatalf("authorization = %q", receivedAuthorization)
	}
}

func TestTranslateWithLLMRequiresConfig(t *testing.T) {
	_, err := translateWithLLM(context.Background(), Config{}, TranslateRequest{Text: "hello", Source: "en", Target: "zh"})
	if err == nil {
		t.Fatal("expected configuration error")
	}
}
