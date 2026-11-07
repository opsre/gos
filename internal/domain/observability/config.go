package observability

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type Config struct {
	Enabled              bool             `json:"enabled"`
	BearerToken          string           `json:"bearer_token"`
	RefreshIntervalSec   int              `json:"refresh_interval_sec"`
	CollectionTimeoutSec int              `json:"collection_timeout_sec"`
	BusinessSources      []BusinessSource `json:"business_sources"`
}

type BusinessSource struct {
	ID            string           `json:"id"`
	ApplicationID string           `json:"application_id"`
	Env           string           `json:"env"`
	URL           string           `json:"url"`
	BearerToken   string           `json:"bearer_token"`
	Metrics       []BusinessMetric `json:"metrics"`
}

type BusinessMetric struct {
	Name   string            `json:"name"`
	Help   string            `json:"help"`
	Type   string            `json:"type"`
	Path   string            `json:"path"`
	Labels map[string]string `json:"labels"`
}

func (c *Config) Defaults() {
	if c.RefreshIntervalSec == 0 {
		c.RefreshIntervalSec = 30
	}
	if c.CollectionTimeoutSec == 0 {
		c.CollectionTimeoutSec = 5
		if c.RefreshIntervalSec > 0 && c.RefreshIntervalSec < c.CollectionTimeoutSec {
			c.CollectionTimeoutSec = c.RefreshIntervalSec
		}
	}
}

var metricName = regexp.MustCompile(`^biz_[a-zA-Z0-9_]+$`)
var labelName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

func (c Config) Validate() error {
	if c.RefreshIntervalSec < 1 || c.RefreshIntervalSec > 3600 {
		return fmt.Errorf("metrics.refresh_interval_sec must be between 1 and 3600")
	}
	if c.CollectionTimeoutSec < 1 || c.CollectionTimeoutSec > c.RefreshIntervalSec {
		return fmt.Errorf("metrics.collection_timeout_sec must be between 1 and refresh_interval_sec")
	}
	if strings.ContainsAny(c.BearerToken, "\r\n\t ") {
		return fmt.Errorf("metrics.bearer_token must not contain whitespace")
	}
	if len(c.BusinessSources) > 32 {
		return fmt.Errorf("metrics.business_sources supports at most 32 sources")
	}
	ids, definitions, series := map[string]bool{}, map[string]string{}, map[string]bool{}
	for _, s := range c.BusinessSources {
		if s.ID == "" || s.ApplicationID == "" || s.Env == "" || len(s.ID) > 128 || len(s.Env) > 120 ||
			s.ID != strings.TrimSpace(s.ID) || s.ApplicationID != strings.TrimSpace(s.ApplicationID) || s.Env != strings.TrimSpace(s.Env) {
			return fmt.Errorf("metrics business source requires id, application_id and env")
		}
		if ids[s.ID] {
			return fmt.Errorf("metrics business source id %q is duplicated", s.ID)
		}
		ids[s.ID] = true
		u, err := url.Parse(s.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("metrics source %q requires an http(s) URL without userinfo or fragment", s.ID)
		}
		if strings.ContainsAny(s.BearerToken, "\r\n") || len(s.Metrics) == 0 || len(s.Metrics) > 64 {
			return fmt.Errorf("metrics source %q requires 1..64 metrics and a valid bearer token", s.ID)
		}
		for _, m := range s.Metrics {
			if !metricName.MatchString(m.Name) || m.Help == "" || (m.Type != "gauge" && m.Type != "counter") {
				return fmt.Errorf("metrics source %q requires biz_ metric names, help and gauge/counter types", s.ID)
			}
			if m.Type == "counter" && !strings.HasSuffix(m.Name, "_total") {
				return fmt.Errorf("metrics counter %q must end in _total", m.Name)
			}
			if m.Path != "" && !strings.HasPrefix(m.Path, "/") {
				return fmt.Errorf("metrics %q path must be a JSON pointer", m.Name)
			}
			for i := 0; i < len(m.Path); i++ {
				if m.Path[i] == '~' {
					if i+1 >= len(m.Path) || (m.Path[i+1] != '0' && m.Path[i+1] != '1') {
						return fmt.Errorf("metrics %q path contains an invalid JSON pointer escape", m.Name)
					}
					i++
				}
			}
			if len(m.Labels) > 8 {
				return fmt.Errorf("metrics %q supports at most 8 static labels", m.Name)
			}
			keys := make([]string, 0, len(m.Labels))
			for key := range m.Labels {
				switch key {
				case "project_id", "application_id", "env", "source_id", "instance", "job", "le", "quantile":
					return fmt.Errorf("metrics label %q is reserved", key)
				}
				if !labelName.MatchString(key) {
					return fmt.Errorf("invalid metrics label %q", key)
				}
				keys = append(keys, key)
			}
			sort.Strings(keys)
			signature := fmt.Sprintf("%q/%q/%q", m.Type, m.Help, keys)
			if previous, ok := definitions[m.Name]; ok && previous != signature {
				return fmt.Errorf("metrics %q has inconsistent type, help or label names", m.Name)
			}
			definitions[m.Name] = signature
			values := make([]string, 0, len(keys))
			for _, key := range keys {
				values = append(values, m.Labels[key])
			}
			identity := fmt.Sprintf("%q/%q/%q", s.ID, m.Name, values)
			if series[identity] {
				return fmt.Errorf("metrics %q has duplicate series in source %q", m.Name, s.ID)
			}
			series[identity] = true
		}
	}
	return nil
}
