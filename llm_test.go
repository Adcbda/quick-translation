package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestTranslateWithLLM(t *testing.T) {
	var receivedPath string
	var receivedAuthorization string
	var receivedStream bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuthorization = r.Header.Get("Authorization")
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		receivedStream, _ = request["stream"].(bool)
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
	if !receivedStream {
		t.Fatal("expected stream=true in LLM request")
	}
}

func TestTranslateWithLLMStreamsChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server does not support flushing")
		}
		for _, event := range []string{
			`{"choices":[{"delta":{"content":"你好"}}]}`,
			`{"choices":[{"delta":{"content":"，世界！"}}]}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	var chunks []string
	got, err := translateWithLLMStream(context.Background(), Config{
		LLMBaseURL: server.URL,
		LLMModel:   "test-model",
	}, TranslateRequest{Text: "Hello, world!", Source: "en", Target: "zh"}, func(delta string) {
		chunks = append(chunks, delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "你好，世界！" {
		t.Fatalf("translation = %q", got)
	}
	if want := []string{"你好", "，世界！"}; !reflect.DeepEqual(chunks, want) {
		t.Fatalf("chunks = %#v, want %#v", chunks, want)
	}
}

func TestTranslateWithLLMRequiresConfig(t *testing.T) {
	_, err := translateWithLLM(context.Background(), Config{}, TranslateRequest{Text: "hello", Source: "en", Target: "zh"})
	if err == nil {
		t.Fatal("expected configuration error")
	}
}
