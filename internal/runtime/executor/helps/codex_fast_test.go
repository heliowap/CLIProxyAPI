package helps

import (
	"bytes"
	"testing"

	"github.com/tidwall/gjson"
)

func TestApplyCodexFastServiceTierSetsPriorityForSupportedFastSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
	}{
		{name: "gpt-5.4", model: "gpt-5.4(fast)"},
		{name: "gpt-5.5 case-insensitive suffix", model: "gpt-5.5(FAST)"},
		{name: "gpt-5.6-sol", model: "gpt-5.6-sol(fast)"},
		{name: "gpt-5.6-terra", model: "gpt-5.6-terra(fast)"},
		{name: "gpt-5.6-luna", model: "gpt-5.6-luna(fast)"},
		{name: "gpt-6-astra", model: "gpt-6-astra(fast)"},
		{name: "gpt-6-astra case-insensitive suffix", model: "gpt-6-astra(FAST)"},
		{name: "fast plus high", model: "gpt-5.6-sol(fast-high)"},
		{name: "high plus fast", model: "gpt-5.6-sol(high-fast)"},
		{name: "fast plus xhigh", model: "gpt-5.6-sol(fast-xhigh)"},
		{name: "fast plus max", model: "gpt-5.6-luna(fast-max)"},
		{name: "gpt-6-astra fast plus low", model: "gpt-6-astra(fast-low)"},
		{name: "gpt-6-astra fast plus medium", model: "gpt-6-astra(fast-medium)"},
		{name: "gpt-6-astra fast plus high", model: "gpt-6-astra(fast-high)"},
		{name: "gpt-6-astra fast plus xhigh", model: "gpt-6-astra(fast-xhigh)"},
		{name: "gpt-6-astra fast plus max", model: "gpt-6-astra(fast-max)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := []byte(`{"model":"gpt-placeholder","input":[]}`)
			out := ApplyCodexFastServiceTier(input, tt.model)
			if got := gjson.GetBytes(out, "service_tier").String(); got != "priority" {
				t.Fatalf("service_tier = %q, want priority", got)
			}
		})
	}
}

func TestApplyCodexFastServiceTierLeavesUnsupportedOrNonFastModelsUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
	}{
		{name: "no suffix", model: "gpt-5.5"},
		{name: "thinking off is not ChatGPT Fast", model: "gpt-5.5(none)"},
		{name: "thinking high is not ChatGPT Fast", model: "gpt-5.5(high)"},
		{name: "mini does not support Fast", model: "gpt-5.4-mini(fast)"},
		{name: "spark does not support Fast", model: "gpt-5.3-codex-spark(fast)"},
		{name: "gpt-6-astra without suffix", model: "gpt-6-astra"},
		{name: "gpt-6-astra effort only is not ChatGPT Fast", model: "gpt-6-astra(xhigh)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := []byte(`{"model":"gpt-placeholder","input":[]}`)
			original := bytes.Clone(input)
			out := ApplyCodexFastServiceTier(input, tt.model)
			if gjson.GetBytes(out, "service_tier").Exists() {
				t.Fatalf("service_tier was set for %q: %s", tt.model, out)
			}
			if !bytes.Equal(out, original) && !bytes.Equal(out, input) {
				t.Fatalf("payload changed for %q: %s", tt.model, out)
			}
		})
	}
}

func TestCodexModelWithoutFastSuffixKeepsEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  string
	}{
		{model: "gpt-5.6-sol", want: "gpt-5.6-sol"},
		{model: "gpt-5.6-sol(high)", want: "gpt-5.6-sol(high)"},
		{model: "gpt-5.6-sol(fast)", want: "gpt-5.6-sol"},
		{model: "gpt-5.6-sol(fast-high)", want: "gpt-5.6-sol(high)"},
		{model: "gpt-5.6-sol(high-fast)", want: "gpt-5.6-sol(high)"},
		{model: "gpt-5.6-sol(FAST-XHIGH)", want: "gpt-5.6-sol(XHIGH)"},
		{model: "gpt-5.6-luna(fast-max)", want: "gpt-5.6-luna(max)"},
		{model: "gpt-6-astra", want: "gpt-6-astra"},
		{model: "gpt-6-astra(low)", want: "gpt-6-astra(low)"},
		{model: "gpt-6-astra(fast)", want: "gpt-6-astra"},
		{model: "gpt-6-astra(fast-low)", want: "gpt-6-astra(low)"},
		{model: "gpt-6-astra(fast-medium)", want: "gpt-6-astra(medium)"},
		{model: "gpt-6-astra(fast-high)", want: "gpt-6-astra(high)"},
		{model: "gpt-6-astra(fast-xhigh)", want: "gpt-6-astra(xhigh)"},
		{model: "gpt-6-astra(fast-max)", want: "gpt-6-astra(max)"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			if got := CodexModelWithoutFastSuffix(tt.model); got != tt.want {
				t.Fatalf("CodexModelWithoutFastSuffix(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestApplyCodexFastServiceTierReusesCanonicalPriority(t *testing.T) {
	t.Parallel()

	input := []byte(`{"model":"gpt-5.5","service_tier":"priority","input":[]}`)
	out := ApplyCodexFastServiceTier(input, "gpt-5.5(fast)")
	if &out[0] != &input[0] {
		t.Fatal("canonical priority caused a payload copy")
	}
}
