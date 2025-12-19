package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Server struct {
	Addr           string `yaml:"addr"`
	ReadTimeoutMS  int    `yaml:"read_timeout_ms"`
	WriteTimeoutMS int    `yaml:"write_timeout_ms"`
	IdleTimeoutMS  int    `yaml:"idle_timeout_ms"`
	MaxBodyBytes   int64  `yaml:"max_body_bytes"`
}

type Observability struct {
	LogLevel       string `yaml:"log_level"`       // "debug","info","warn","error"
	PrometheusPath string `yaml:"prometheus_path"` // e.g. "/metrics"
}

type Limits struct {
	Default struct {
		RequestsPerMinute int `yaml:"requests_per_minute"`
		Burst             int `yaml:"burst"`
	} `yaml:"default"`
}

type APIKey struct {
	ID       string            `yaml:"id"`
	Secret   string            `yaml:"secret"`
	Metadata map[string]string `yaml:"metadata"`
}

type Auth struct {
	Header string   `yaml:"header"`
	Keys   []APIKey `yaml:"keys"`
}

type Routes struct {
	ID    string `yaml:"id"`
	Match struct {
		PathPrefix string   `yaml:"path_prefix"`
		Methods    []string `yaml:"methods"`
	} `yaml:"match"`

	Upstream struct {
		URL       string `yaml:"url"`
		TimeoutMS int    `yaml:"timeout_ms"`
	} `yaml:"upstream"`
}

type Root struct {
	Server        Server        `yaml:"server"`
	Observability Observability `yaml:"observability"`
	Auth          Auth          `yaml:"auth"`
	Limits        Limits        `yaml:"limits"`
	Routes        []Routes      `yaml:"routes"`
}

func (s Server) ReadTimeout() time.Duration {
	if s.ReadTimeoutMS == 0 {
		return 5 * time.Second
	}
	return time.Duration(s.ReadTimeoutMS) * time.Millisecond
}

func (s Server) WriteTimeout() time.Duration {
	if s.WriteTimeoutMS == 0 {
		return 10 * time.Second
	}
	return time.Duration(s.WriteTimeoutMS) * time.Millisecond
}

func (s Server) IdleTimeout() time.Duration {
	if s.IdleTimeoutMS == 0 {
		return 60 * time.Second
	}
	return time.Duration(s.IdleTimeoutMS) * time.Millisecond
}

func (s Server) MaxBody() int64 {
	if s.MaxBodyBytes == 0 {
		return 10 << 20
	}
	return s.MaxBodyBytes
} // default 10MB

func Load(path string) (*Root, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Root
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].Upstream.TimeoutMS <= 0 {
			cfg.Routes[i].Upstream.TimeoutMS = 3000
		}
	}
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Observability.LogLevel == "" {
		cfg.Observability.LogLevel = "info"
	}
	if cfg.Auth.Header == "" {
		cfg.Auth.Header = "X-API-Key"
	}
	if cfg.Limits.Default.RequestsPerMinute <= 0 {
		cfg.Limits.Default.RequestsPerMinute = 60
	}
	if cfg.Limits.Default.Burst <= 0 {
		cfg.Limits.Default.Burst = 30
	}

	return &cfg, nil
}
