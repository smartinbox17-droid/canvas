package platform

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                      string
	DataDir                   string
	DatabaseDriver            string
	DatabaseURL               string
	RedisURL                  string
	AutoMigrate               bool
	RegistrationEnabled       bool
	AllowPrivateUpstreams     bool
	AllowedPrivateHosts       []string
	CORSOrigins               []string
	PublicBaseURL             string
	WorkerConcurrency         int
	ChannelConcurrency        int
	ShutdownTimeout           time.Duration
	EnableProviderPlugins     bool
	AgentURL                  string
	AgentToken                string
}

func LoadConfigFromEnv() Config {
	return Config{
		Addr:                  envOr("CANVAS_BACKEND_ADDR", ":8080"),
		DataDir:               envOr("CANVAS_DATA_PATH", envOr("CANVAS_BACKEND_DATA_DIR", "./data")),
		DatabaseDriver:        strings.ToLower(envOr("CANVAS_DATABASE_DRIVER", "sqlite")),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		RedisURL:              os.Getenv("REDIS_URL"),
		AutoMigrate:           envBool("CANVAS_AUTO_MIGRATE", true),
		RegistrationEnabled:   envBool("CANVAS_REGISTRATION_ENABLED", false),
		AllowPrivateUpstreams: envBool("CANVAS_ALLOW_PRIVATE_UPSTREAMS", false),
		AllowedPrivateHosts:   splitCSV(os.Getenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS")),
		CORSOrigins:           splitCSV(envOr("CANVAS_CORS_ORIGINS", "http://localhost:3000")),
		PublicBaseURL:         os.Getenv("CANVAS_PUBLIC_BASE_URL"),
		WorkerConcurrency:     envInt("CANVAS_WORKER_CONCURRENCY", 3),
		ChannelConcurrency:    envInt("CANVAS_CHANNEL_CONCURRENCY", 3),
		ShutdownTimeout:       envDuration("CANVAS_SHUTDOWN_TIMEOUT", 10*time.Minute),
		EnableProviderPlugins: envBool("ENABLE_PROVIDER_PLUGINS", false),
		AgentURL:              envOr("YINGCE_AGENT_URL", "http://localhost:8090"),
		AgentToken:            os.Getenv("YINGCE_AGENT_TOKEN"),
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func splitCSV(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
