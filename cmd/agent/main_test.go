package main

import "testing"

// TestResolveAgentName 解析上下文数据，得到后续流程需要的结果。
func TestResolveAgentName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		configuredName string
		hostname       string
		hostIP         string
		want           string
	}{
		{
			name:           "prefer configured name",
			configuredName: "custom-agent",
			hostname:       "prod-node-01",
			hostIP:         "10.5.5.5",
			want:           "custom-agent",
		},
		{
			name:     "build from hostname and ipv4",
			hostname: "prod-node-01",
			hostIP:   "10.5.5.5",
			want:     "prod-node-01-10-5-5-5",
		},
		{
			name:     "fallback to hostname only",
			hostname: "prod-node-01",
			want:     "prod-node-01",
		},
		{
			name:   "fallback to ip only",
			hostIP: "10.5.5.5",
			want:   "agent-10-5-5-5",
		},
		{
			name:   "support ipv6 formatting",
			hostIP: "fe80::1",
			want:   "agent-fe80-1",
		},
		{
			name: "fallback to generic agent name",
			want: "agent",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveAgentName(tc.configuredName, tc.hostname, tc.hostIP); got != tc.want {
				t.Fatalf("resolveAgentName(%q, %q, %q) = %q, want %q", tc.configuredName, tc.hostname, tc.hostIP, got, tc.want)
			}
		})
	}
}

// TestFormatAgentNameIP 封装当前模块的业务处理逻辑。
func TestFormatAgentNameIP(t *testing.T) {
	t.Parallel()

	if got := formatAgentNameIP("10.5.5.5"); got != "10-5-5-5" {
		t.Fatalf("formatAgentNameIP ipv4 = %q, want %q", got, "10-5-5-5")
	}
	if got := formatAgentNameIP("  "); got != "" {
		t.Fatalf("formatAgentNameIP blank = %q, want empty", got)
	}
}
