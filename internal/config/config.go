// Package config — единственное место в сервисе, где читается окружение.
// Всё остальное получает уже разобранную и проверенную конфигурацию.
// Соблюдение проверяет `make guard`.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ServiceName string `envconfig:"SERVICE_NAME" default:"chrome_skill"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`

	HTTPPort  int `envconfig:"PORT" default:"8080"`
	DebugPort int `envconfig:"DEBUG_PORT" default:"8081"`

	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"30s"`

	CORSOrigins []string `envconfig:"CORS_ORIGINS" default:"http://localhost:5173"`
	// Каталог собранного фронта: монолит раздаёт SPA с диска.
	StaticDir string `envconfig:"STATIC_DIR" default:"front/public"`

	Database Database
}

type Database struct {
	URL             string        `envconfig:"DATABASE_URL" required:"true"`
	MaxConns        int32         `envconfig:"DATABASE_MAX_CONNS" default:"10"`
	ConnMaxLifetime time.Duration `envconfig:"DATABASE_CONN_MAX_LIFETIME" default:"30m"`
}

// Load читает .env, если он есть, и затем окружение: переменная процесса всегда
// сильнее файла, поэтому в контейнере файл просто не нужен.
//
// Путь к .env берётся здесь же, а не в main: окружение читается только в этом
// пакете, и на это есть проверка в `make guard`.
func Load() (*Config, error) {
	if dotenv := os.Getenv("DOTENV"); dotenv != "" {
		_ = godotenv.Load(dotenv)
	}

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("чтение конфигурации: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.HTTPPort == c.DebugPort {
		return fmt.Errorf("порты HTTP=%d и DEBUG=%d должны различаться", c.HTTPPort, c.DebugPort)
	}
	if strings.TrimSpace(c.StaticDir) == "" {
		return fmt.Errorf("STATIC_DIR не задан")
	}
	return nil
}

func (c *Config) HTTPAddr() string  { return fmt.Sprintf(":%d", c.HTTPPort) }
func (c *Config) DebugAddr() string { return fmt.Sprintf(":%d", c.DebugPort) }
