package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

// TestConnectionPoolInitialization tests that the connection pool is created correctly
func TestConnectionPoolInitialization(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	r := &Redis{
		redisAddress:   s.Addr(),
		connectTimeout: 100,
		readTimeout:    100,
		Ttl:            300,
	}

	r.Connect()

	if r.Pool == nil {
		t.Fatal("expected non-nil connection pool")
	}

	// Test that we can get a connection
	conn := r.Pool.Get()
	if conn == nil {
		t.Fatal("expected non-nil connection from pool")
	}
	defer conn.Close()

	if conn.Err() != nil {
		t.Errorf("connection has error: %v", conn.Err())
	}
}

// TestConnectionWithPassword tests connection with authentication
func TestConnectionWithPassword(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Set password on miniredis
	s.RequireAuth("secret123")

	r := &Redis{
		redisAddress:  s.Addr(),
		redisPassword: "secret123",
		Ttl:           300,
	}

	r.Connect()

	if r.Pool == nil {
		t.Fatal("expected non-nil connection pool")
	}

	// Test that authenticated connection works
	conn := r.Pool.Get()
	defer conn.Close()

	// Try a simple command
	_, err := conn.Do("PING")
	if err != nil {
		t.Errorf("authenticated connection failed: %v", err)
	}
}

// TestConnectionWithWrongPassword tests handling of authentication failures
func TestConnectionWithWrongPassword(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.RequireAuth("correct_password")

	r := &Redis{
		redisAddress:  s.Addr(),
		redisPassword: "wrong_password",
		Ttl:           300,
	}

	r.Connect()

	// Connection pool is created, but operations should fail
	conn := r.Pool.Get()
	defer conn.Close()

	_, err := conn.Do("PING")
	if err == nil {
		t.Error("expected authentication error, got nil")
	}
}

// TestKeyCount tests the DBSIZE tracking
func TestKeyCount(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()

	// Initially empty
	count := r.KeyCount()
	if count != 0 {
		t.Errorf("expected 0 keys, got %d", count)
	}

	// Add some keys
	s.Set("key1", "value1")
	s.Set("key2", "value2")
	s.HSet("zone1.", "host1", "data")

	count = r.KeyCount()
	if count != 3 {
		t.Errorf("expected 3 keys, got %d", count)
	}

	// Delete a key
	s.Del("key1")

	count = r.KeyCount()
	if count != 2 {
		t.Errorf("expected 2 keys after deletion, got %d", count)
	}
}

// TestLoadZonesWithEmptyDatabase tests zone loading from empty Redis
func TestLoadZonesWithEmptyDatabase(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	if len(r.Zones) != 0 {
		t.Errorf("expected 0 zones, got %d", len(r.Zones))
	}
}

// TestLoadZonesWithData tests zone loading with actual zones
func TestLoadZonesWithData(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add some test zones
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.HSet("example.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)
	s.HSet("example.org.", "www", `{"a":[{"ttl":300,"ip":"9.10.11.12"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	if len(r.Zones) != 3 {
		t.Errorf("expected 3 zones, got %d: %v", len(r.Zones), r.Zones)
	}

	// Verify zone names
	expectedZones := map[string]bool{
		"example.com.": true,
		"example.net.": true,
		"example.org.": true,
	}

	for _, zone := range r.Zones {
		if !expectedZones[zone] {
			t.Errorf("unexpected zone: %s", zone)
		}
	}
}

// TestLoadZonesWithPrefix tests zone loading with key prefix
func TestLoadZonesWithPrefix(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add zones with and without prefix
	s.HSet("dns:example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.HSet("dns:example.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)
	s.HSet("other:example.org.", "@", `{"a":[{"ttl":300,"ip":"9.10.11.12"}]}`)
	s.HSet("example.test.", "@", `{"a":[{"ttl":300,"ip":"13.14.15.16"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		keyPrefix:    "dns:",
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Should only load zones with "dns:" prefix
	if len(r.Zones) != 2 {
		t.Errorf("expected 2 zones with prefix, got %d: %v", len(r.Zones), r.Zones)
	}

	for _, zone := range r.Zones {
		if zone != "example.com." && zone != "example.net." {
			t.Errorf("unexpected zone without prefix: %s", zone)
		}
	}
}

// TestLoadZonesWithSuffix tests zone loading with key suffix
func TestLoadZonesWithSuffix(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add zones with and without suffix
	s.HSet("example.com.zone", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.HSet("example.net.zone", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)
	s.HSet("example.org.", "@", `{"a":[{"ttl":300,"ip":"9.10.11.12"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		keySuffix:    ".zone",
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Should only load zones with ".zone" suffix and strip the suffix
	if len(r.Zones) != 2 {
		t.Errorf("expected 2 zones with suffix, got %d: %v", len(r.Zones), r.Zones)
	}

	// Suffix should be stripped from zone names
	// After stripping ".zone" from "example.com.zone" we get "example.com."
	// But Redis key was "example.com.zone", so after strip we get "example.com"
	for _, zone := range r.Zones {
		if zone != "example.com" && zone != "example.net" {
			t.Errorf("unexpected zone (suffix should be stripped): %s (zones: %v)", zone, r.Zones)
		}
	}
}

// TestLoadZonesWithPrefixAndSuffix tests zone loading with both prefix and suffix
func TestLoadZonesWithPrefixAndSuffix(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add zones with various combinations
	s.HSet("prod:example.com.:v1", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.HSet("prod:example.net.:v1", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)
	s.HSet("prod:example.org.", "@", `{"a":[{"ttl":300,"ip":"9.10.11.12"}]}`)      // no suffix
	s.HSet("dev:example.test.:v1", "@", `{"a":[{"ttl":300,"ip":"13.14.15.16"}]}`) // wrong prefix

	r := &Redis{
		redisAddress: s.Addr(),
		keyPrefix:    "prod:",
		keySuffix:    ":v1",
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Should only load zones with both "prod:" prefix and ":v1" suffix
	// Both prefix and suffix should be stripped
	if len(r.Zones) != 2 {
		t.Errorf("expected 2 zones with prefix and suffix, got %d: %v", len(r.Zones), r.Zones)
	}

	// After stripping both "prod:" prefix and ":v1" suffix
	for _, zone := range r.Zones {
		if zone != "example.com." && zone != "example.net." {
			t.Errorf("unexpected zone (prefix and suffix should be stripped): %s", zone)
		}
	}
}

// TestServeDNSWithConnectionFailure tests DNS query handling when Redis is down
func TestServeDNSWithConnectionFailure(t *testing.T) {
	// Create Redis instance with invalid address
	r := &Redis{
		redisAddress: "invalid:9999",
		Ttl:          300,
		Zones:        []string{"example.com."},
	}

	r.Connect()

	// Create DNS query
	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	// Should handle connection failure gracefully (return SERVFAIL)
	code, err := r.ServeDNS(ctx, rec, m)

	// The plugin should return success (code 0) but response should have SERVFAIL rcode
	if err != nil {
		t.Errorf("expected no error from ServeDNS, got: %v", err)
	}

	if code != 0 {
		t.Errorf("expected code 0, got %d", code)
	}

	// Response should indicate server failure
	if rec.Msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("expected SERVFAIL rcode, got %d", rec.Msg.Rcode)
	}
}

// TestServeDNSWithMalformedJSON tests handling of corrupted zone data
func TestServeDNSWithMalformedJSON(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add zone with malformed JSON
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}`)  // missing closing braces
	s.HSet("example.com.", "bad", `not json at all`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	// Query for host with malformed JSON
	m := new(dns.Msg)
	m.SetQuestion("bad.example.com.", dns.TypeA)

	code, err := r.ServeDNS(ctx, rec, m)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if code != 0 {
		t.Errorf("expected code 0, got %d", code)
	}

	// Should return SERVFAIL for malformed JSON
	if rec.Msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("expected SERVFAIL for malformed JSON, got rcode %d", rec.Msg.Rcode)
	}
}

// TestZoneRefreshOnDBSizeChange tests that zones reload when DBSIZE changes
func TestZoneRefreshOnDBSizeChange(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Start with one zone
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	if len(r.Zones) != 1 {
		t.Fatalf("expected 1 zone initially, got %d", len(r.Zones))
	}

	// Add another zone
	s.HSet("example.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)

	// Trigger zone refresh by making a DNS query
	// The ServeDNS function checks if DBSIZE changed
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()
	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeA)

	r.ServeDNS(ctx, rec, m)

	// Zones should have been reloaded
	if len(r.Zones) != 2 {
		t.Errorf("expected 2 zones after DBSIZE change, got %d: %v", len(r.Zones), r.Zones)
	}
}

// TestZoneRefreshOnTimeExpiry tests that zones reload after time threshold
func TestZoneRefreshOnTimeExpiry(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Start with one zone
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)

	r := &Redis{
		redisAddress:   s.Addr(),
		Ttl:            300,
		LastZoneUpdate: time.Now().Add(-11 * time.Minute), // Simulate old update
	}

	r.Connect()
	r.LoadZones()

	initialUpdateTime := r.LastZoneUpdate

	// Add another zone
	s.HSet("example.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)

	// Trigger zone refresh by making a DNS query
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()
	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeA)

	r.ServeDNS(ctx, rec, m)

	// LastZoneUpdate should have been updated
	if !r.LastZoneUpdate.After(initialUpdateTime) {
		t.Error("expected LastZoneUpdate to be refreshed after time expiry")
	}

	// Zones should have been reloaded
	if len(r.Zones) != 2 {
		t.Errorf("expected 2 zones after time-based refresh, got %d: %v", len(r.Zones), r.Zones)
	}
}

// TestConnectionPoolTimeout tests connection timeout behavior
func TestConnectionPoolTimeout(t *testing.T) {
	// This test documents timeout behavior, actual timeout testing is difficult
	// without a real slow network
	r := &Redis{
		redisAddress:   "localhost:6379",
		connectTimeout: 1, // 1ms timeout
		readTimeout:    1,
		Ttl:            300,
	}

	r.Connect()

	if r.Pool == nil {
		t.Fatal("expected non-nil pool even with timeouts configured")
	}

	// Pool is created, but operations may fail due to timeouts
	// This is expected behavior
}

// TestQueryWithValidRecord tests end-to-end DNS query with valid record
func TestQueryWithValidRecord(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add a valid zone with A record
	s.HSet("example.com.", "www", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Create DNS query
	m := new(dns.Msg)
	m.SetQuestion("www.example.com.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	code, err := r.ServeDNS(ctx, rec, m)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Errorf("expected success code, got %d", code)
	}

	if rec.Msg.Rcode != dns.RcodeSuccess {
		t.Errorf("expected success rcode, got %d", rec.Msg.Rcode)
	}

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(rec.Msg.Answer))
	}

	// Verify the A record
	if aRecord, ok := rec.Msg.Answer[0].(*dns.A); ok {
		if aRecord.A.String() != "1.2.3.4" {
			t.Errorf("expected IP 1.2.3.4, got %s", aRecord.A.String())
		}
	} else {
		t.Error("expected A record in answer")
	}
}

// TestQueryForNonExistentRecord tests NXDOMAIN response
func TestQueryForNonExistentRecord(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add a zone but not the queried host
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Query for non-existent host
	m := new(dns.Msg)
	m.SetQuestion("nonexistent.example.com.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	code, err := r.ServeDNS(ctx, rec, m)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Errorf("expected success code, got %d", code)
	}

	// Should return NXDOMAIN
	if rec.Msg.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN, got rcode %d", rec.Msg.Rcode)
	}
}
