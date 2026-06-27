package main

import (
	"encoding/base64"
	"strconv"
	"testing"
)

func TestEnvInt(t *testing.T) {
	t.Run("with env var set", func(t *testing.T) {
		t.Setenv("TEST_ENV_INT", "42")
		if got := envInt("TEST_ENV_INT", 10); got != 42 {
			t.Errorf("envInt() = %d, want %d", got, 42)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		if got := envInt("TEST_ENV_INT_NOT_SET", 10); got != 10 {
			t.Errorf("envInt() = %d, want %d", got, 10)
		}
	})

	t.Run("zero value", func(t *testing.T) {
		t.Setenv("TEST_ENV_INT_ZERO", "0")
		if got := envInt("TEST_ENV_INT_ZERO", 10); got != 0 {
			t.Errorf("envInt() = %d, want %d", got, 0)
		}
	})

	t.Run("negative value", func(t *testing.T) {
		t.Setenv("TEST_ENV_INT_NEG", "-5")
		if got := envInt("TEST_ENV_INT_NEG", 10); got != -5 {
			t.Errorf("envInt() = %d, want %d", got, -5)
		}
	})
}

func TestEnvInt64(t *testing.T) {
	t.Run("with env var set", func(t *testing.T) {
		t.Setenv("TEST_ENV_INT64", "9223372036854775807")
		if got := envInt64("TEST_ENV_INT64", 100); got != 9223372036854775807 {
			t.Errorf("envInt64() = %d, want %d", got, int64(9223372036854775807))
		}
	})

	t.Run("fallback", func(t *testing.T) {
		if got := envInt64("TEST_ENV_INT64_NOT_SET", 100); got != 100 {
			t.Errorf("envInt64() = %d, want %d", got, 100)
		}
	})
}

func TestGenerateJWTSecret(t *testing.T) {
	t.Run("generates valid base64 secret", func(t *testing.T) {
		secret, err := generateJWTSecret()
		if err != nil {
			t.Fatalf("generateJWTSecret() error = %v", err)
		}

		if len(secret) != 44 {
			t.Errorf("generateJWTSecret() length = %d, want 44", len(secret))
		}

		decoded, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			t.Errorf("generateJWTSecret() = %q is not valid base64: %v", secret, err)
		}
		if len(decoded) != 32 {
			t.Errorf("generateJWTSecret() decoded length = %d, want 32", len(decoded))
		}
	})

	t.Run("generates different secrets each time", func(t *testing.T) {
		s1, _ := generateJWTSecret()
		s2, _ := generateJWTSecret()
		if s1 == s2 {
			t.Error("generateJWTSecret() returned the same value twice")
		}
	})
}

func TestParseS3URL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    *S3Config
		wantErr bool
	}{
		{
			name: "full URL with query params",
			url:  "s3://mybucket?region=us-west-2&endpoint=http://localhost:9000&access_key=admin&secret_key=password",
			want: &S3Config{
				Bucket:    "mybucket",
				Region:    "us-west-2",
				Endpoint:  "http://localhost:9000",
				AccessKey: "admin",
				SecretKey: "password",
			},
			wantErr: false,
		},
		{
			name: "minimal URL with default region",
			url:  "s3://mybucket",
			want: &S3Config{
				Bucket: "mybucket",
				Region: "us-east-1",
			},
			wantErr: false,
		},
		{
			name: "with user info credentials",
			url:  "s3://admin:password@mybucket?region=ap-southeast-1",
			want: &S3Config{
				Bucket:    "mybucket",
				Region:    "ap-southeast-1",
				AccessKey: "admin",
				SecretKey: "password",
			},
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			url:     "http://mybucket",
			wantErr: true,
		},
		{
			name:    "missing bucket",
			url:     "s3://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseS3URL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseS3URL(%q) error = %v, wantErr = %v", tt.url, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Bucket != tt.want.Bucket {
				t.Errorf("ParseS3URL(%q).Bucket = %q, want %q", tt.url, got.Bucket, tt.want.Bucket)
			}
			if got.Region != tt.want.Region {
				t.Errorf("ParseS3URL(%q).Region = %q, want %q", tt.url, got.Region, tt.want.Region)
			}
			if got.Endpoint != tt.want.Endpoint {
				t.Errorf("ParseS3URL(%q).Endpoint = %q, want %q", tt.url, got.Endpoint, tt.want.Endpoint)
			}
			if got.AccessKey != tt.want.AccessKey {
				t.Errorf("ParseS3URL(%q).AccessKey = %q, want %q", tt.url, got.AccessKey, tt.want.AccessKey)
			}
			if got.SecretKey != tt.want.SecretKey {
				t.Errorf("ParseS3URL(%q).SecretKey = %q, want %q", tt.url, got.SecretKey, tt.want.SecretKey)
			}
		})
	}
}

func TestEnvString(t *testing.T) {
	t.Run("with env var set", func(t *testing.T) {
		t.Setenv("TEST_ENV_STRING", "hello")
		if got := envString("TEST_ENV_STRING", "fallback"); got != "hello" {
			t.Errorf("envString() = %q, want %q", got, "hello")
		}
	})

	t.Run("fallback", func(t *testing.T) {
		if got := envString("TEST_ENV_STRING_NOT_SET", "fallback"); got != "fallback" {
			t.Errorf("envString() = %q, want %q", got, "fallback")
		}
	})

	t.Run("empty env var returns empty", func(t *testing.T) {
		t.Setenv("TEST_ENV_STRING_EMPTY", "")
		if got := envString("TEST_ENV_STRING_EMPTY", "fallback"); got != "" {
			t.Errorf("envString() = %q, want %q", got, "")
		}
	})
}

func TestConfigValidate(t *testing.T) {
	validCfg := &Config{
		NodeID:               1,
		JWTSecret:            "my-secret-key",
		Port:                 ":8080",
		DatabaseURL:          "postgres://localhost:5432/vortex",
		BCryptCost:           10,
		SegmentDurationMs:    10000,
		SegmentSize:          131072,
		MessageRetentionDays: 7,
		DefaultPageSize:      20,
		MaxPageSize:          100,
	}

	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{name: "valid config", cfg: validCfg, wantErr: false},
		{
			name:    "negative node id",
			cfg:     func() *Config { c := *validCfg; c.NodeID = -1; return &c }(),
			wantErr: true, errMsg: "NODE_ID is required",
		},
		{
			name:    "node id exceeds max",
			cfg:     func() *Config { c := *validCfg; c.NodeID = 32; return &c }(),
			wantErr: true, errMsg: "NODE_ID must be between 0 and 31",
		},
		{
			name:    "bcrypt cost too low",
			cfg:     func() *Config { c := *validCfg; c.BCryptCost = 4; return &c }(),
			wantErr: true, errMsg: "BCRYPT_COST must be between 10 and 15",
		},
		{
			name:    "bcrypt cost too high",
			cfg:     func() *Config { c := *validCfg; c.BCryptCost = 16; return &c }(),
			wantErr: true, errMsg: "BCRYPT_COST must be between 10 and 15",
		},
		{
			name:    "empty jwt secret",
			cfg:     func() *Config { c := *validCfg; c.JWTSecret = ""; return &c }(),
			wantErr: true, errMsg: "JWT_SECRET is required",
		},
		{
			name:    "empty port",
			cfg:     func() *Config { c := *validCfg; c.Port = ""; return &c }(),
			wantErr: true, errMsg: "PORT is required",
		},
		{
			name:    "invalid port non-numeric",
			cfg:     func() *Config { c := *validCfg; c.Port = ":abc"; return &c }(),
			wantErr: true, errMsg: "PORT must be a valid port number",
		},
		{
			name:    "port out of range (zero)",
			cfg:     func() *Config { c := *validCfg; c.Port = ":0"; return &c }(),
			wantErr: true, errMsg: "PORT must be a valid port number",
		},
		{
			name:    "port too large",
			cfg:     func() *Config { c := *validCfg; c.Port = ":65536"; return &c }(),
			wantErr: true, errMsg: "PORT must be a valid port number",
		},
		{
			name:    "port without colon prefix",
			cfg:     func() *Config { c := *validCfg; c.Port = "8080"; return &c }(),
			wantErr: true, errMsg: "PORT must be a valid port number",
		},
		{
			name:    "empty database url",
			cfg:     func() *Config { c := *validCfg; c.DatabaseURL = ""; return &c }(),
			wantErr: true, errMsg: "DATABASE_URL is required",
		},
		{
			name:    "segment duration zero",
			cfg:     func() *Config { c := *validCfg; c.SegmentDurationMs = 0; return &c }(),
			wantErr: true, errMsg: "SEGMENT_DURATION_MS must be positive",
		},
		{
			name:    "segment size zero",
			cfg:     func() *Config { c := *validCfg; c.SegmentSize = 0; return &c }(),
			wantErr: true, errMsg: "SEGMENT_SIZE must be positive",
		},
		{
			name:    "retention days zero",
			cfg:     func() *Config { c := *validCfg; c.MessageRetentionDays = 0; return &c }(),
			wantErr: true, errMsg: "MESSAGE_RETENTION_DAYS must be positive",
		},
		{
			name:    "default page size zero",
			cfg:     func() *Config { c := *validCfg; c.DefaultPageSize = 0; return &c }(),
			wantErr: true, errMsg: "PAGE_SIZE must be positive",
		},
		{
			name:    "max page size zero",
			cfg:     func() *Config { c := *validCfg; c.MaxPageSize = 0; return &c }(),
			wantErr: true, errMsg: "PAGE_SIZE must be positive",
		},
		{
			name:    "default exceeds max",
			cfg:     func() *Config { c := *validCfg; c.DefaultPageSize = 200; c.MaxPageSize = 100; return &c }(),
			wantErr: true, errMsg: "DEFAULT_PAGE_SIZE cannot exceed MAX_PAGE_SIZE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if err.Error()[:len(tt.errMsg)] != tt.errMsg {
					t.Errorf("Config.Validate() error = %q, want prefix %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestEnvInt64EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want int64
	}{
		{"max int64", "9223372036854775807", 9223372036854775807},
		{"min int64", "-9223372036854775808", -9223372036854775808},
		{"zero", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := strconv.ParseInt(tt.val, 10, 64)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			t.Setenv("TEST_ENV_INT64_EC", tt.val)
			if got := envInt64("TEST_ENV_INT64_EC", 0); got != parsed {
				t.Errorf("envInt64() = %d, want %d", got, parsed)
			}
		})
	}
}
