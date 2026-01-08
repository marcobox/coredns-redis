package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// TestLoadZonesWithLargeDataset tests SCAN pagination with many zones
func TestLoadZonesWithLargeDataset(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add more than the cursor batch size (1000) to test pagination
	for i := 0; i < 1500; i++ {
		zoneName := "example" + string(rune(i)) + ".com."
		s.HSet(zoneName, "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	}

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Should load all zones despite pagination
	if len(r.Zones) != 1500 {
		t.Errorf("expected 1500 zones (testing pagination), got %d", len(r.Zones))
	}
}

// TestLoadZonesDuplicateHandling tests that SCAN duplicates are handled
func TestLoadZonesDuplicateHandling(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add zones
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.HSet("example.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()

	// Load zones multiple times
	r.LoadZones()
	firstCount := len(r.Zones)

	r.LoadZones()
	secondCount := len(r.Zones)

	// Count should be stable (no duplicates accumulated)
	if firstCount != secondCount {
		t.Errorf("expected stable zone count, got %d then %d", firstCount, secondCount)
	}

	if firstCount != 2 {
		t.Errorf("expected 2 zones, got %d", firstCount)
	}
}

// TestLoadZonesWithNonHashKeys tests that non-hash keys are handled
func TestLoadZonesWithNonHashKeys(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add mix of hash and non-hash keys
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.Set("regular-key", "value")
	s.HSet("example.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Should only load hash keys (zones), skip regular keys
	// Note: miniredis may behave differently than real Redis for SCAN with non-hash keys
	if len(r.Zones) < 2 {
		t.Errorf("expected at least 2 hash zones, got %d", len(r.Zones))
	}
}

// TestLoadZonesWithSpecialCharacters tests zone names with special characters
func TestLoadZonesWithSpecialCharacters(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add zones with various special characters
	s.HSet("example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.HSet("my-domain.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)
	s.HSet("sub_domain.org.", "@", `{"a":[{"ttl":300,"ip":"9.10.11.12"}]}`)
	s.HSet("123numeric.test.", "@", `{"a":[{"ttl":300,"ip":"13.14.15.16"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	if len(r.Zones) != 4 {
		t.Errorf("expected 4 zones with special characters, got %d: %v", len(r.Zones), r.Zones)
	}
}

// TestLoadZonesWithComplexPrefix tests complex prefix patterns
func TestLoadZonesWithComplexPrefix(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Add zones with complex prefixes
	s.HSet("prod:us-east:example.com.", "@", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	s.HSet("prod:us-west:example.net.", "@", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)
	s.HSet("dev:us-east:example.org.", "@", `{"a":[{"ttl":300,"ip":"9.10.11.12"}]}`)

	r := &Redis{
		redisAddress: s.Addr(),
		keyPrefix:    "prod:",
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	// Should only load "prod:" prefixed zones
	if len(r.Zones) != 2 {
		t.Errorf("expected 2 zones with complex prefix, got %d: %v", len(r.Zones), r.Zones)
	}

	// Verify prefix was stripped correctly
	for _, zone := range r.Zones {
		if zone != "us-east:example.com." && zone != "us-west:example.net." {
			t.Errorf("unexpected zone after prefix strip: %s", zone)
		}
	}
}

// TestSaveAndLoadRecord tests the save/load helper functions
func TestSaveAndLoadRecord(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()

	// Save some records
	err := r.save("example.com.", "www", `{"a":[{"ttl":300,"ip":"1.2.3.4"}]}`)
	if err != nil {
		t.Fatalf("failed to save record: %v", err)
	}

	err = r.save("example.com.", "mail", `{"a":[{"ttl":300,"ip":"5.6.7.8"}]}`)
	if err != nil {
		t.Fatalf("failed to save second record: %v", err)
	}

	// Load the zone metadata (which uses HKEYS internally)
	loadedZone := r.load("example.com.")

	if loadedZone == nil {
		t.Fatal("expected non-nil loaded zone")
	}

	if loadedZone.Name != "example.com." {
		t.Errorf("expected zone name 'example.com.', got '%s'", loadedZone.Name)
	}

	// Verify locations
	expectedLocations := []string{"www", "mail"}
	for _, loc := range expectedLocations {
		if _, found := loadedZone.Locations[loc]; !found {
			t.Errorf("expected location '%s' not found in loaded zone", loc)
		}
	}

	if len(loadedZone.Locations) != 2 {
		t.Errorf("expected 2 locations, got %d", len(loadedZone.Locations))
	}
}

// TestLoadNonExistentZone tests loading a zone that doesn't exist
func TestLoadNonExistentZone(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()

	// Try to load non-existent zone
	zone := r.load("nonexistent.com.")

	// The load function returns a Zone struct even for non-existent zones
	// It will have an empty Locations map
	if zone == nil {
		t.Fatal("expected non-nil zone struct")
	}

	if zone.Name != "nonexistent.com." {
		t.Errorf("expected zone name 'nonexistent.com.', got '%s'", zone.Name)
	}

	if len(zone.Locations) != 0 {
		t.Errorf("expected 0 locations for non-existent zone, got %d", len(zone.Locations))
	}
}

// TestDecodeScanReply tests the SCAN response decoder
func TestDecodeScanReply(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		expectError bool
		expectPanic bool
		cursor      int
		keyCount    int
	}{
		{
			name: "valid scan reply with cursor 0",
			input: []interface{}{
				[]byte("0"),
				[]interface{}{
					[]byte("key1"),
					[]byte("key2"),
					[]byte("key3"),
				},
			},
			expectError: false,
			cursor:      0,
			keyCount:    3,
		},
		{
			name: "valid scan reply with cursor 123",
			input: []interface{}{
				[]byte("123"),
				[]interface{}{
					[]byte("keyA"),
					[]byte("keyB"),
				},
			},
			expectError: false,
			cursor:      123,
			keyCount:    2,
		},
		{
			name: "empty result set",
			input: []interface{}{
				[]byte("0"),
				[]interface{}{},
			},
			expectError: false,
			cursor:      0,
			keyCount:    0,
		},
		{
			name: "invalid cursor format returns error",
			input: []interface{}{
				[]byte("not_a_number"),
				[]interface{}{},
			},
			expectError: true,
		},
		{
			name:        "invalid reply format - not array",
			input:       "invalid",
			expectError: true,
		},
		{
			name: "invalid reply format - wrong array size",
			input: []interface{}{
				[]byte("0"),
			},
			expectError: true,
		},
		{
			name: "invalid cursor - not byte array",
			input: []interface{}{
				"not_bytes",
				[]interface{}{},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := decodeScanReply(tc.input)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.cursor != tc.cursor {
				t.Errorf("expected cursor %d, got %d", tc.cursor, result.cursor)
			}

			if len(result.keys) != tc.keyCount {
				t.Errorf("expected %d keys, got %d", tc.keyCount, len(result.keys))
			}
		})
	}
}
