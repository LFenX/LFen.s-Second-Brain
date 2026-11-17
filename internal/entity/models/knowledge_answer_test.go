package models

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ragflow/internal/common"
)

type answerTransport func(*http.Request) (*http.Response, error)

func (f answerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestKnowledgeAnswerPrivateDriverKeepsOpaqueKeyAndFixedCall(t *testing.T) {
	key, base, region, name := "sk-synthetic:opaque+/=final", "https://api.deepseek.com", "default", "deepseek-flash"
	driver := NewDeepSeekModel(map[string]string{"default": base}, URLSuffix{Chat: "/chat/completions"})
	chat, err := NewPrivateDeepSeekChat(driver, &name, &APIConfig{ApiKey: &key, BaseURL: &base, Region: &region}, nil)
	if err != nil {
		t.Fatal(err)
	}
	private := chat.ModelDriver.(*DeepSeekModel)
	if private == driver || private.baseModel.httpClient == driver.baseModel.httpClient || *chat.APIConfig.ApiKey != key {
		t.Fatal("shared driver or truncated opaque credential")
	}
	calls := 0
	client := *private.baseModel.httpClient
	client.Transport = answerTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Header.Get("Authorization") != "Bearer "+key || req.URL.String() != "https://api.deepseek.com/chat/completions" {
			t.Fatal("opaque key or destination changed")
		}
		var body map[string]any
		if json.NewDecoder(req.Body).Decode(&body) != nil || body["model"] != name || body["stream"] != false || body["max_tokens"] != float64(1024) || body["temperature"] != 0.1 || body["thinking"].(map[string]any)["type"] != "disabled" {
			t.Fatal("fixed non-streaming limits changed")
		}
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://another.example.invalid/collect"}}, Body: io.NopCloser(strings.NewReader("synthetic redirect")), Request: req}, nil
	})
	private.baseModel.httpClient = &client
	maxTokens, temperature, thinking := 1024, 0.1, false
	_, err = chat.ChatWithMessages(context.Background(), []Message{{Role: "user", Content: "synthetic-only"}}, &ChatConfig{MaxTokens: &maxTokens, Temperature: &temperature, Thinking: &thinking}, &common.ModelUsage{})
	if err == nil || calls != 1 {
		t.Fatal("redirect/retry sent another provider request", calls, err)
	}
}
