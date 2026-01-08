package redis

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

// ========== CAA RECORD TESTS ==========

// TestCAARecordIssue tests CAA record with issue tag
func TestCAARecordIssue(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.HSet("example.com.", "www", `{
		"caa":[{"flag":0,"tag":"issue","value":"letsencrypt.org"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("www.example.com.", dns.TypeCAA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	code, err := r.ServeDNS(ctx, rec, m)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Errorf("expected success code, got %d", code)
	}

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 CAA record, got %d", len(rec.Msg.Answer))
	}

	caaRecord, ok := rec.Msg.Answer[0].(*dns.CAA)
	if !ok {
		t.Fatal("expected CAA record")
	}

	if caaRecord.Flag != 0 {
		t.Errorf("expected flag 0, got %d", caaRecord.Flag)
	}

	if caaRecord.Tag != "issue" {
		t.Errorf("expected tag 'issue', got '%s'", caaRecord.Tag)
	}

	if caaRecord.Value != "letsencrypt.org" {
		t.Errorf("expected value 'letsencrypt.org', got '%s'", caaRecord.Value)
	}
}

// TestCAARecordIssuewild tests CAA record with issuewild tag
func TestCAARecordIssuewild(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.HSet("example.com.", "@", `{
		"caa":[{"flag":0,"tag":"issuewild","value":"comodoca.com"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeCAA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 CAA record, got %d", len(rec.Msg.Answer))
	}

	caaRecord := rec.Msg.Answer[0].(*dns.CAA)

	if caaRecord.Tag != "issuewild" {
		t.Errorf("expected tag 'issuewild', got '%s'", caaRecord.Tag)
	}

	if caaRecord.Value != "comodoca.com" {
		t.Errorf("expected value 'comodoca.com', got '%s'", caaRecord.Value)
	}
}

// TestCAARecordIodef tests CAA record with iodef tag
func TestCAARecordIodef(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.HSet("example.com.", "@", `{
		"caa":[{"flag":0,"tag":"iodef","value":"mailto:security@example.com"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeCAA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 CAA record, got %d", len(rec.Msg.Answer))
	}

	caaRecord := rec.Msg.Answer[0].(*dns.CAA)

	if caaRecord.Tag != "iodef" {
		t.Errorf("expected tag 'iodef', got '%s'", caaRecord.Tag)
	}

	if caaRecord.Value != "mailto:security@example.com" {
		t.Errorf("expected value 'mailto:security@example.com', got '%s'", caaRecord.Value)
	}
}

// TestCAARecordCriticalFlag tests CAA record with critical flag (128)
func TestCAARecordCriticalFlag(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.HSet("example.com.", "@", `{
		"caa":[{"flag":128,"tag":"issue","value":"ca.example.net"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeCAA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 CAA record, got %d", len(rec.Msg.Answer))
	}

	caaRecord := rec.Msg.Answer[0].(*dns.CAA)

	if caaRecord.Flag != 128 {
		t.Errorf("expected flag 128 (critical), got %d", caaRecord.Flag)
	}
}

// TestCAAMultipleRecords tests multiple CAA records
func TestCAAMultipleRecords(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.HSet("example.com.", "@", `{
		"caa":[
			{"flag":0,"tag":"issue","value":"letsencrypt.org"},
			{"flag":0,"tag":"issue","value":"comodoca.com"},
			{"flag":0,"tag":"iodef","value":"mailto:security@example.com"}
		]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeCAA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 3 {
		t.Fatalf("expected 3 CAA records, got %d", len(rec.Msg.Answer))
	}

	// Verify all are CAA records
	for i, rr := range rec.Msg.Answer {
		if _, ok := rr.(*dns.CAA); !ok {
			t.Errorf("answer %d is not a CAA record", i)
		}
	}
}

// TestCAAEmptyValue tests CAA record with empty value (should be skipped)
func TestCAAEmptyValue(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.HSet("example.com.", "@", `{
		"caa":[
			{"flag":0,"tag":"issue","value":""},
			{"flag":0,"tag":"issue","value":"letsencrypt.org"}
		]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeCAA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	// Should only get 1 record (empty value is skipped)
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 CAA record (empty skipped), got %d", len(rec.Msg.Answer))
	}

	caaRecord := rec.Msg.Answer[0].(*dns.CAA)
	if caaRecord.Value == "" {
		t.Error("empty value record should have been skipped")
	}
}

// TestCAAEmptyTag tests CAA record with empty tag (should be skipped)
func TestCAAEmptyTag(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	s.HSet("example.com.", "@", `{
		"caa":[
			{"flag":0,"tag":"","value":"letsencrypt.org"},
			{"flag":0,"tag":"issue","value":"comodoca.com"}
		]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeCAA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	// Should only get 1 record (empty tag is skipped)
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 CAA record (empty tag skipped), got %d", len(rec.Msg.Answer))
	}

	caaRecord := rec.Msg.Answer[0].(*dns.CAA)
	if caaRecord.Tag == "" {
		t.Error("empty tag record should have been skipped")
	}
}

// ========== SPLIT255 TESTS (TXT Records) ==========

// TestTXTRecordExactly254Chars tests TXT record with 254 characters (no split)
func TestTXTRecordExactly254Chars(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Create a 254 character string (split255 splits at >= 255)
	txt254 := strings.Repeat("a", 254)

	s.HSet("example.com.", "test", `{
		"txt":[{"ttl":300,"text":"`+txt254+`"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeTXT)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 TXT record, got %d", len(rec.Msg.Answer))
	}

	txtRecord := rec.Msg.Answer[0].(*dns.TXT)

	// Should be single string (no split needed for < 255)
	if len(txtRecord.Txt) != 1 {
		t.Errorf("expected 1 TXT string (no split), got %d", len(txtRecord.Txt))
	}

	if txtRecord.Txt[0] != txt254 {
		t.Errorf("TXT content mismatch, got length %d", len(txtRecord.Txt[0]))
	}
}

// TestTXTRecordExactly255Chars tests TXT record with exactly 255 characters (requires split)
func TestTXTRecordExactly255Chars(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Create a 255 character string (split255 splits at >= 255)
	txt255 := strings.Repeat("a", 255)

	s.HSet("example.com.", "test", `{
		"txt":[{"ttl":300,"text":"`+txt255+`"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeTXT)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 TXT record, got %d", len(rec.Msg.Answer))
	}

	txtRecord := rec.Msg.Answer[0].(*dns.TXT)

	// Should be split into 2 strings at >= 255
	if len(txtRecord.Txt) != 2 {
		t.Errorf("expected 2 TXT strings (split at 255), got %d", len(txtRecord.Txt))
	}

	// Verify total length
	combined := strings.Join(txtRecord.Txt, "")
	if len(combined) != 255 {
		t.Errorf("expected combined length 255, got %d", len(combined))
	}
}

// TestTXTRecord256Chars tests TXT record with 256 characters (requires split)
func TestTXTRecord256Chars(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Create a 256 character string
	txt256 := strings.Repeat("b", 256)

	s.HSet("example.com.", "test", `{
		"txt":[{"ttl":300,"text":"`+txt256+`"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeTXT)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 TXT record, got %d", len(rec.Msg.Answer))
	}

	txtRecord := rec.Msg.Answer[0].(*dns.TXT)

	// Should be split into 2 strings (255 + 1)
	if len(txtRecord.Txt) != 2 {
		t.Errorf("expected 2 TXT strings (split at 255), got %d", len(txtRecord.Txt))
	}

	// Verify total length
	combined := strings.Join(txtRecord.Txt, "")
	if len(combined) != 256 {
		t.Errorf("expected combined length 256, got %d", len(combined))
	}
}

// TestTXTRecord400Chars tests TXT record with 400 characters (large but fits in DNS UDP)
// Note: DNS UDP has a 512 byte packet size limit. TXT records > ~400 chars may be
// truncated without EDNS0 support, which is standard DNS behavior.
func TestTXTRecord400Chars(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Create a 400 character string (fits in 512 byte DNS UDP packet)
	txt400 := strings.Repeat("c", 400)

	jsonData := fmt.Sprintf(`{"txt":[{"ttl":300,"text":"%s"}]}`, txt400)
	s.HSet("example.com.", "test", jsonData)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeTXT)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	code, err := r.ServeDNS(ctx, rec, m)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 TXT record, got %d", len(rec.Msg.Answer))
	}

	txtRecord := rec.Msg.Answer[0].(*dns.TXT)

	// Should be split into 2 strings (255 + 145)
	expectedParts := 2
	if len(txtRecord.Txt) != expectedParts {
		t.Errorf("expected %d TXT strings, got %d", expectedParts, len(txtRecord.Txt))
	}

	// Verify total length
	combined := strings.Join(txtRecord.Txt, "")
	if len(combined) != 400 {
		t.Errorf("expected combined length 400, got %d", len(combined))
	}

	// Verify first part is exactly 255
	if len(txtRecord.Txt[0]) != 255 {
		t.Errorf("expected first part length 255, got %d", len(txtRecord.Txt[0]))
	}
}

// TestTXTRecord1000CharsWithEDNS0 tests TXT record with 1000 characters using EDNS0
// EDNS0 allows DNS responses larger than the 512 byte UDP limit
func TestTXTRecord1000CharsWithEDNS0(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Create a 1000 character string
	txt1000 := strings.Repeat("d", 1000)

	jsonData := fmt.Sprintf(`{"txt":[{"ttl":300,"text":"%s"}]}`, txt1000)
	s.HSet("example.com.", "test", jsonData)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300,
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeTXT)
	// Enable EDNS0 to allow packets larger than 512 bytes
	m.SetEdns0(4096, false)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	code, err := r.ServeDNS(ctx, rec, m)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != dns.RcodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 TXT record, got %d", len(rec.Msg.Answer))
	}

	txtRecord := rec.Msg.Answer[0].(*dns.TXT)

	// Should be split into 4 strings (255 + 255 + 255 + 235)
	expectedParts := 4
	if len(txtRecord.Txt) != expectedParts {
		t.Errorf("expected %d TXT strings, got %d", expectedParts, len(txtRecord.Txt))
	}

	// Verify total length
	combined := strings.Join(txtRecord.Txt, "")
	if len(combined) != 1000 {
		t.Errorf("expected combined length 1000, got %d", len(combined))
	}

	// Verify each segment is <= 255
	for i, segment := range txtRecord.Txt {
		if len(segment) > 255 {
			t.Errorf("segment %d has length %d (should be <= 255)", i, len(segment))
		}
	}

	// Verify first 3 parts are exactly 255
	for i := 0; i < 3; i++ {
		if len(txtRecord.Txt[i]) != 255 {
			t.Errorf("expected part %d length 255, got %d", i, len(txtRecord.Txt[i]))
		}
	}
}

// ========== MINTTL TESTS ==========

// TestMinTTLBothZero tests minTtl when both TTLs are zero
func TestMinTTLBothZero(t *testing.T) {
	r := &Redis{Ttl: 0}

	result := r.minTtl(0)

	if result != defaultTtl {
		t.Errorf("expected default TTL %d when both zero, got %d", defaultTtl, result)
	}
}

// TestMinTTLOnlyRedisTTLZero tests minTtl when only redis.Ttl is zero
func TestMinTTLOnlyRedisTTLZero(t *testing.T) {
	r := &Redis{Ttl: 0}

	result := r.minTtl(600)

	if result != 600 {
		t.Errorf("expected record TTL 600 when redis.Ttl is zero, got %d", result)
	}
}

// TestMinTTLOnlyRecordTTLZero tests minTtl when only record TTL is zero
func TestMinTTLOnlyRecordTTLZero(t *testing.T) {
	r := &Redis{Ttl: 300}

	result := r.minTtl(0)

	if result != 300 {
		t.Errorf("expected redis.Ttl 300 when record TTL is zero, got %d", result)
	}
}

// TestMinTTLRedisTTLLower tests minTtl when redis.Ttl < record TTL
func TestMinTTLRedisTTLLower(t *testing.T) {
	r := &Redis{Ttl: 100}

	result := r.minTtl(500)

	if result != 100 {
		t.Errorf("expected lower TTL 100, got %d", result)
	}
}

// TestMinTTLRecordTTLLower tests minTtl when record TTL < redis.Ttl
func TestMinTTLRecordTTLLower(t *testing.T) {
	r := &Redis{Ttl: 500}

	result := r.minTtl(100)

	if result != 100 {
		t.Errorf("expected lower TTL 100, got %d", result)
	}
}

// TestMinTTLEqual tests minTtl when both TTLs are equal
func TestMinTTLEqual(t *testing.T) {
	r := &Redis{Ttl: 300}

	result := r.minTtl(300)

	if result != 300 {
		t.Errorf("expected TTL 300 when equal, got %d", result)
	}
}

// TestMinTTLVeryLargeTTL tests minTtl with very large TTL values
func TestMinTTLVeryLargeTTL(t *testing.T) {
	r := &Redis{Ttl: 86400} // 1 day

	result := r.minTtl(604800) // 1 week

	if result != 86400 {
		t.Errorf("expected lower TTL 86400, got %d", result)
	}
}

// TestMinTTLIntegration tests minTtl with real DNS query
func TestMinTTLIntegration(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	// Record TTL is 600, Redis TTL is 300
	s.HSet("example.com.", "test", `{
		"a":[{"ttl":600,"ip":"1.2.3.4"}]
	}`)

	r := &Redis{
		redisAddress: s.Addr(),
		Ttl:          300, // Redis TTL is lower
	}

	r.Connect()
	r.LoadZones()

	m := new(dns.Msg)
	m.SetQuestion("test.example.com.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	ctx := context.Background()

	r.ServeDNS(ctx, rec, m)

	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("expected 1 A record, got %d", len(rec.Msg.Answer))
	}

	// TTL in response should be the minimum (300)
	if rec.Msg.Answer[0].Header().Ttl != 300 {
		t.Errorf("expected TTL 300 (min), got %d", rec.Msg.Answer[0].Header().Ttl)
	}
}
