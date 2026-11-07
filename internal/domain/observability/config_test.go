package observability

import "testing"

func TestBusinessSourceConfigRejectsConflictingDefinitionsAndReservedLabels(t *testing.T) {
	base := func() Config {
		return Config{RefreshIntervalSec: 30, CollectionTimeoutSec: 5, BusinessSources: []BusinessSource{{ID: "source", ApplicationID: "app", Env: "prod", URL: "http://example.invalid/stats", Metrics: []BusinessMetric{{Name: "biz_pending", Help: "Pending.", Type: "gauge", Path: "/pending"}}}}}
	}
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"duplicate source", func(c *Config) { c.BusinessSources = append(c.BusinessSources, c.BusinessSources[0]) }},
		{"reserved label", func(c *Config) { c.BusinessSources[0].Metrics[0].Labels = map[string]string{"env": "other"} }},
		{"counter suffix", func(c *Config) { c.BusinessSources[0].Metrics[0].Type = "counter" }},
		{"invalid path", func(c *Config) { c.BusinessSources[0].Metrics[0].Path = "pending.value" }},
		{"invalid pointer escape", func(c *Config) { c.BusinessSources[0].Metrics[0].Path = "/pending~2value" }},
		{"credentials in URL", func(c *Config) { c.BusinessSources[0].URL = "http://user:secret@example.invalid/stats" }},
		{"duplicate series", func(c *Config) {
			c.BusinessSources[0].Metrics = append(c.BusinessSources[0].Metrics, c.BusinessSources[0].Metrics[0])
		}},
		{"conflicting metadata", func(c *Config) {
			m := c.BusinessSources[0].Metrics[0]
			m.Help = "Different help."
			m.Labels = map[string]string{"result": "other"}
			c.BusinessSources[0].Metrics = append(c.BusinessSources[0].Metrics, m)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.change(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected config rejection")
			}
		})
	}
	if err := base().Validate(); err != nil {
		t.Fatal(err)
	}
}
