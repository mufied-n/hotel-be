package platform

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestConfig_EnvironmentAndOverrides(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		wantEnv      string
		wantPort     string
		wantHold     time.Duration
		wantOutbox   time.Duration
		isDev        bool
		isProd       bool
	}{
		{
			name:       "defaults when no env set",
			envVars:    map[string]string{},
			wantEnv:    "development",
			wantPort:   "8080",
			wantHold:   30 * time.Minute,
			wantOutbox: 2 * time.Second,
			isDev:      true,
			isProd:     false,
		},
		{
			name: "custom environment variables",
			envVars: map[string]string{
				"APP_ENV":         "production",
				"APP_PORT":        "9000",
				"HOLD_TIMEOUT":    "15m",
				"OUTBOX_INTERVAL": "500ms",
			},
			wantEnv:    "production",
			wantPort:   "9000",
			wantHold:   15 * time.Minute,
			wantOutbox: 500 * time.Millisecond,
			isDev:      false,
			isProd:     true,
		},
		{
			name: "invalid duration fallbacks to defaults",
			envVars: map[string]string{
				"HOLD_TIMEOUT":    "invalid-duration",
				"OUTBOX_INTERVAL": "-5s",
			},
			wantEnv:    "development",
			wantPort:   "8080",
			wantHold:   30 * time.Minute,
			wantOutbox: 2 * time.Second,
			isDev:      true,
			isProd:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set
			keys := []string{"APP_ENV", "APP_PORT", "HOLD_TIMEOUT", "OUTBOX_INTERVAL"}
			for _, k := range keys {
				_ = os.Unsetenv(k)
			}
			for k, v := range tt.envVars {
				_ = os.Setenv(k, v)
			}
			defer func() {
				for _, k := range keys {
					_ = os.Unsetenv(k)
				}
			}()

			cfg := LoadConfig()
			if cfg.Environment != tt.wantEnv {
				t.Errorf("Environment = %s, want %s", cfg.Environment, tt.wantEnv)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %s, want %s", cfg.Port, tt.wantPort)
			}
			if cfg.HoldTimeout != tt.wantHold {
				t.Errorf("HoldTimeout = %v, want %v", cfg.HoldTimeout, tt.wantHold)
			}
			if cfg.OutboxInterval != tt.wantOutbox {
				t.Errorf("OutboxInterval = %v, want %v", cfg.OutboxInterval, tt.wantOutbox)
			}
			if cfg.IsDevelopment() != tt.isDev {
				t.Errorf("IsDevelopment() = %v, want %v", cfg.IsDevelopment(), tt.isDev)
			}
			if cfg.IsProduction() != tt.isProd {
				t.Errorf("IsProduction() = %v, want %v", cfg.IsProduction(), tt.isProd)
			}
		})
	}
}

func TestNewValkeyAndLogger(t *testing.T) {
	cfg := Config{ValkeyAddr: "localhost:6379"}
	client := NewValkey(cfg)
	if client == nil {
		t.Fatal("NewValkey returned nil")
	}
	_ = client.Close()

	logger := NewLogger()
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	_, err := NewDB(context.Background(), Config{DatabaseDSN: "invalid://dsn"})
	if err == nil {
		t.Error("expected error from NewDB with invalid dsn, got nil")
	}
}

func TestConfig_Validate(t *testing.T) {
	validCfg := Config{
		Environment:    "production",
		Port:           "8080",
		DatabaseDSN:    "postgres://localhost:5432/booking",
		ValkeyAddr:     "localhost:6379",
		HoldTimeout:    15 * time.Minute,
		OutboxInterval: 1 * time.Second,
	}

	tests := []struct {
		name    string
		modify  func(c *Config)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "empty port",
			modify:  func(c *Config) { c.Port = "" },
			wantErr: true,
		},
		{
			name:    "empty database DSN",
			modify:  func(c *Config) { c.DatabaseDSN = "" },
			wantErr: true,
		},
		{
			name:    "empty valkey addr",
			modify:  func(c *Config) { c.ValkeyAddr = "" },
			wantErr: true,
		},
		{
			name:    "zero hold timeout",
			modify:  func(c *Config) { c.HoldTimeout = 0 },
			wantErr: true,
		},
		{
			name:    "negative hold timeout",
			modify:  func(c *Config) { c.HoldTimeout = -5 * time.Minute },
			wantErr: true,
		},
		{
			name:    "zero outbox interval",
			modify:  func(c *Config) { c.OutboxInterval = 0 },
			wantErr: true,
		},
		{
			name:    "negative outbox interval",
			modify:  func(c *Config) { c.OutboxInterval = -1 * time.Second },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validCfg
			tt.modify(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
