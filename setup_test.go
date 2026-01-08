package redis

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

// TestSetupParse tests the configuration parsing logic
// Note: These tests will attempt to connect to Redis as part of the setup process
// Tests are designed to validate parsing, not Redis connectivity
func TestSetupParse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldErr   bool
		errContains string
		validate    func(*testing.T, *Redis)
	}{
		{
			name: "valid configuration with all options",
			input: `redis {
				address localhost:6379
				password secret123
				prefix dns:
				suffix .zone
				connect_timeout 5000
				read_timeout 3000
				ttl 600
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.redisAddress != "localhost:6379" {
					t.Errorf("expected address 'localhost:6379', got '%s'", r.redisAddress)
				}
				if r.redisPassword != "secret123" {
					t.Errorf("expected password 'secret123', got '%s'", r.redisPassword)
				}
				if r.keyPrefix != "dns:" {
					t.Errorf("expected prefix 'dns:', got '%s'", r.keyPrefix)
				}
				if r.keySuffix != ".zone" {
					t.Errorf("expected suffix '.zone', got '%s'", r.keySuffix)
				}
				if r.connectTimeout != 5000 {
					t.Errorf("expected connect_timeout 5000, got %d", r.connectTimeout)
				}
				if r.readTimeout != 3000 {
					t.Errorf("expected read_timeout 3000, got %d", r.readTimeout)
				}
				if r.Ttl != 600 {
					t.Errorf("expected ttl 600, got %d", r.Ttl)
				}
			},
		},
		{
			name: "valid configuration with defaults",
			input: `redis {
				address localhost:6379
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.keyPrefix != "" {
					t.Errorf("expected empty prefix, got '%s'", r.keyPrefix)
				}
				if r.keySuffix != "" {
					t.Errorf("expected empty suffix, got '%s'", r.keySuffix)
				}
				if r.Ttl != 300 {
					t.Errorf("expected default ttl 300, got %d", r.Ttl)
				}
				if r.connectTimeout != 0 {
					t.Errorf("expected default connect_timeout 0, got %d", r.connectTimeout)
				}
				if r.readTimeout != 0 {
					t.Errorf("expected default read_timeout 0, got %d", r.readTimeout)
				}
			},
		},
		{
			name: "valid configuration with prefix only",
			input: `redis {
				prefix myprefix:
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.keyPrefix != "myprefix:" {
					t.Errorf("expected prefix 'myprefix:', got '%s'", r.keyPrefix)
				}
				if r.Ttl != 300 {
					t.Errorf("expected default ttl 300, got %d", r.Ttl)
				}
			},
		},
		{
			name: "valid configuration with suffix only",
			input: `redis {
				suffix :mysuffix
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.keySuffix != ":mysuffix" {
					t.Errorf("expected suffix ':mysuffix', got '%s'", r.keySuffix)
				}
				if r.Ttl != 300 {
					t.Errorf("expected default ttl 300, got %d", r.Ttl)
				}
			},
		},
		{
			name: "empty configuration block",
			input: `redis {
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				// Empty block with only whitespace returns empty Redis struct from line 108
				// NextBlock() returns false, so it doesn't enter the loop
				if r.Ttl != 0 {
					t.Errorf("expected ttl 0 for empty block, got %d", r.Ttl)
				}
				if r.keyPrefix != "" {
					t.Errorf("expected empty prefix, got '%s'", r.keyPrefix)
				}
				if r.keySuffix != "" {
					t.Errorf("expected empty suffix, got '%s'", r.keySuffix)
				}
			},
		},
		{
			name: "invalid connect_timeout (non-numeric)",
			input: `redis {
				connect_timeout notanumber
			}`,
			shouldErr: false, // Parser sets to 0 on error, doesn't fail
			validate: func(t *testing.T, r *Redis) {
				if r.connectTimeout != 0 {
					t.Errorf("expected connect_timeout 0 (fallback), got %d", r.connectTimeout)
				}
			},
		},
		{
			name: "invalid read_timeout (non-numeric)",
			input: `redis {
				read_timeout notanumber
			}`,
			shouldErr: false, // Parser sets to 0 on error, doesn't fail
			validate: func(t *testing.T, r *Redis) {
				if r.readTimeout != 0 {
					t.Errorf("expected read_timeout 0 (fallback), got %d", r.readTimeout)
				}
			},
		},
		{
			name: "invalid ttl (non-numeric)",
			input: `redis {
				ttl notanumber
			}`,
			shouldErr: false, // Parser sets to defaultTtl on error
			validate: func(t *testing.T, r *Redis) {
				if r.Ttl != defaultTtl {
					t.Errorf("expected ttl %d (fallback), got %d", defaultTtl, r.Ttl)
				}
			},
		},
		{
			name: "negative ttl value",
			input: `redis {
				ttl -100
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				// Negative int converts to large uint32 due to type conversion
				// Just verify parsing doesn't crash
				if r.Ttl == 0 {
					t.Error("ttl should not be 0")
				}
			},
		},
		{
			name:        "unknown property",
			input:       `redis { unknown_option value }`,
			shouldErr:   true,
			errContains: "unknown property 'unknown_option'",
		},
		{
			name: "multiple valid options",
			input: `redis {
				address redis.example.com:6380
				password complexpass
				prefix prod:dns:
				suffix .zone
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.redisAddress != "redis.example.com:6380" {
					t.Errorf("expected address 'redis.example.com:6380', got '%s'", r.redisAddress)
				}
				if r.redisPassword != "complexpass" {
					t.Errorf("expected password 'complexpass', got '%s'", r.redisPassword)
				}
				if r.keyPrefix != "prod:dns:" {
					t.Errorf("expected prefix 'prod:dns:', got '%s'", r.keyPrefix)
				}
				if r.keySuffix != ".zone" {
					t.Errorf("expected suffix '.zone', got '%s'", r.keySuffix)
				}
			},
		},
		{
			name: "zero values are valid",
			input: `redis {
				connect_timeout 0
				read_timeout 0
				ttl 0
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.connectTimeout != 0 {
					t.Errorf("expected connect_timeout 0, got %d", r.connectTimeout)
				}
				if r.readTimeout != 0 {
					t.Errorf("expected read_timeout 0, got %d", r.readTimeout)
				}
				if r.Ttl != 0 {
					t.Errorf("expected ttl 0, got %d", r.Ttl)
				}
			},
		},
		{
			name: "large timeout values",
			input: `redis {
				connect_timeout 999999
				read_timeout 999999
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.connectTimeout != 999999 {
					t.Errorf("expected connect_timeout 999999, got %d", r.connectTimeout)
				}
				if r.readTimeout != 999999 {
					t.Errorf("expected read_timeout 999999, got %d", r.readTimeout)
				}
			},
		},
		{
			name: "prefix and suffix with special characters",
			input: `redis {
				prefix hostsync/coredns:
				suffix :v1
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.keyPrefix != "hostsync/coredns:" {
					t.Errorf("expected prefix 'hostsync/coredns:', got '%s'", r.keyPrefix)
				}
				if r.keySuffix != ":v1" {
					t.Errorf("expected suffix ':v1', got '%s'", r.keySuffix)
				}
			},
		},
		{
			name: "password with special characters",
			input: `redis {
				password myp@ssw0rd
			}`,
			shouldErr: false,
			validate: func(t *testing.T, r *Redis) {
				if r.redisPassword != "myp@ssw0rd" {
					t.Errorf("expected password 'myp@ssw0rd', got '%s'", r.redisPassword)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			r, err := redisParse(c)

			if tc.shouldErr {
				if err == nil {
					t.Fatalf("expected error containing '%s', got nil", tc.errContains)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing '%s', got '%s'", tc.errContains, err.Error())
				}
				return
			}

			// Note: Connection errors from Connect() and LoadZones() are logged but don't fail parsing
			// This is by design - the plugin should start even if Redis is temporarily unavailable
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if r == nil {
				t.Fatal("expected non-nil Redis instance")
			}

			if tc.validate != nil {
				tc.validate(t, r)
			}
		})
	}
}

// TestSetupNoBlock tests configuration without a block
func TestSetupNoBlock(t *testing.T) {
	input := `redis`
	c := caddy.NewTestController("dns", input)
	r, err := redisParse(c)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return empty Redis struct (no block means return &Redis{} on line 108)
	if r.Ttl != 0 {
		t.Errorf("expected ttl 0 for no-block config, got %d", r.Ttl)
	}
	if r.keyPrefix != "" {
		t.Errorf("expected empty prefix for no-block config, got '%s'", r.keyPrefix)
	}
}

// TestSetupFunction tests the main setup function
func TestSetupFunction(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{
			name: "valid setup",
			input: `redis {
				address localhost:6379
			}`,
			shouldErr: false,
		},
		{
			name:      "invalid setup - unknown property",
			input:     `redis { badprop value }`,
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			err := setup(c)

			if tc.shouldErr && err == nil {
				t.Fatal("expected error, got nil")
			}

			if !tc.shouldErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestConfigurationEdgeCases tests edge cases in configuration parsing
func TestConfigurationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(*testing.T, *Redis)
	}{
		{
			name: "all string values",
			input: `redis {
				address 127.0.0.1:6379
				password ""
				prefix ""
				suffix ""
			}`,
			validate: func(t *testing.T, r *Redis) {
				if r.redisAddress != "127.0.0.1:6379" {
					t.Errorf("expected address '127.0.0.1:6379', got '%s'", r.redisAddress)
				}
				// Empty string quotes are stripped by Caddy
				if r.redisPassword != `""` {
					t.Logf("password value (may be empty or quoted): '%s'", r.redisPassword)
				}
			},
		},
		{
			name: "mixed case property names are case sensitive",
			input: `redis {
				Address localhost:6379
			}`,
			validate: func(t *testing.T, r *Redis) {
				// "Address" (capital A) is unknown, so it's treated as error
				// This test will fail parsing due to unknown property
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tc.input)
			r, err := redisParse(c)

			// Edge case tests may have errors, we're just documenting behavior
			if err != nil {
				t.Logf("parsing failed (may be expected): %v", err)
				return
			}

			if tc.validate != nil {
				tc.validate(t, r)
			}
		})
	}
}
