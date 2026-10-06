package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestCodexFastSuffixPayloadBarrier(t *testing.T) {
	cases := []struct {
		name            string
		payload         config.PayloadConfig
		wantServiceTier string
	}{
		{name: "no-rule", wantServiceTier: `"priority"`},
		{
			name: "override",
			payload: config.PayloadConfig{Override: []config.PayloadRule{{
				Models: []config.PayloadModelRule{{Name: "gpt-6.1-sol"}},
				Params: map[string]any{"service_tier": "default"},
			}}},
			wantServiceTier: `"default"`,
		},
		{
			name: "filter",
			payload: config.PayloadConfig{Filter: []config.PayloadFilterRule{{
				Models: []config.PayloadModelRule{{Name: "gpt-6.1-sol"}},
				Params: []string{"service_tier"},
			}}},
		},
	}
	for _, transport := range []string{"http", "websocket"} {
		for _, stream := range []bool{false, true} {
			for _, tc := range cases {
				t.Run(transport+map[bool]string{false: "/execute/", true: "/stream/"}[stream]+tc.name, func(t *testing.T) {
					captured := make(chan []byte, 1)
					completed := []byte(`{"type":"response.completed","response":{"id":"resp_fast_barrier","status":"completed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if transport == "websocket" {
							upgrader := websocket.Upgrader{}
							conn, errUpgrade := upgrader.Upgrade(w, r, nil)
							if errUpgrade != nil {
								t.Error(errUpgrade)
								return
							}
							defer func() {
								if errClose := conn.Close(); errClose != nil {
									t.Error(errClose)
								}
							}()
							_, body, errRead := conn.ReadMessage()
							if errRead != nil {
								t.Error(errRead)
								return
							}
							captured <- body
							if errWrite := conn.WriteMessage(websocket.TextMessage, completed); errWrite != nil {
								t.Error(errWrite)
							}
							return
						}
						body, errRead := io.ReadAll(r.Body)
						if errRead != nil {
							t.Error(errRead)
							return
						}
						captured <- body
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = w.Write(append(append([]byte("data: "), completed...), '\n', '\n'))
					}))
					defer server.Close()
					cfg := &config.Config{Payload: tc.payload}
					var executor cliproxyauth.ProviderExecutor = NewCodexExecutor(cfg)
					if transport == "websocket" {
						executor = NewCodexWebsocketsExecutor(cfg)
					}
					auth := &cliproxyauth.Auth{ID: t.Name(), Provider: "codex", Attributes: map[string]string{"api_key": "test", "base_url": server.URL, "plan_type": "pro"}}
					req := cliproxyexecutor.Request{Model: "gpt-6.1-sol(fast-high)", Payload: []byte(`{"input":"hello"}`)}
					opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse}
					if stream {
						result, errStream := executor.ExecuteStream(context.Background(), auth, req, opts)
						if errStream != nil {
							t.Fatal(errStream)
						}
						for chunk := range result.Chunks {
							if chunk.Err != nil {
								t.Fatal(chunk.Err)
							}
						}
					} else if _, errExecute := executor.Execute(context.Background(), auth, req, opts); errExecute != nil {
						t.Fatal(errExecute)
					}
					body := <-captured
					if got := gjson.GetBytes(body, "service_tier").Raw; got != tc.wantServiceTier {
						t.Fatalf("service_tier = %q, want %q: %s", got, tc.wantServiceTier, body)
					}
					if got := gjson.GetBytes(body, "model").String(); got != "gpt-6.1-sol" {
						t.Fatalf("model = %q, want gpt-6.1-sol: %s", got, body)
					}
					if got := gjson.GetBytes(body, "reasoning.effort").String(); got != "high" {
						t.Fatalf("reasoning.effort = %q, want high: %s", got, body)
					}
				})
			}
		}
	}
}
