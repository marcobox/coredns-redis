package redis

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

var zones = []string{
	"example.com.", "example.net.", "example.test.",
}

var lookupEntries = [][][]string{
	// Example.com
	{
		{"@",
			"{\"soa\":{\"ttl\":300, \"minttl\":100, \"mbox\":\"hostmaster.example.com.\",\"ns\":\"ns1.example.com.\",\"refresh\":44,\"retry\":55,\"expire\":66}}",
		},
		{"x",
			"{\"a\":[{\"ttl\":300, \"ip\":\"1.2.3.4\"},{\"ttl\":300, \"ip\":\"5.6.7.8\"}]," +
				"\"aaaa\":[{\"ttl\":300, \"ip\":\"::1\"}]," +
				"\"txt\":[{\"ttl\":300, \"text\":\"foo\"},{\"ttl\":300, \"text\":\"bar\"}]," +
				"\"ns\":[{\"ttl\":300, \"host\":\"ns1.example.com.\"},{\"ttl\":300, \"host\":\"ns2.example.com.\"}]," +
				"\"mx\":[{\"ttl\":300, \"host\":\"mx1.example.com.\", \"preference\":10},{\"ttl\":300, \"host\":\"mx2.example.com.\", \"preference\":10}]}",
		},
		{"y",
			"{\"cname\":[{\"ttl\":300, \"host\":\"x.example.com.\"}]}",
		},
		{"ns1",
			"{\"a\":[{\"ttl\":300, \"ip\":\"2.2.2.2\"}]}",
		},
		{"ns2",
			"{\"a\":[{\"ttl\":300, \"ip\":\"3.3.3.3\"}]}",
		},
		{"_sip._tcp",
			"{\"srv\":[{\"ttl\":300, \"target\":\"sip.example.com.\",\"port\":555,\"priority\":10,\"weight\":100}]}",
		},
		{"sip",
			"{\"a\":[{\"ttl\":300, \"ip\":\"7.7.7.7\"}]," +
				"\"aaaa\":[{\"ttl\":300, \"ip\":\"::1\"}]}",
		},
	},
	// Example.net
	{
		{"@",
			"{\"soa\":{\"ttl\":300, \"minttl\":100, \"mbox\":\"hostmaster.example.net.\",\"ns\":\"ns1.example.net.\",\"refresh\":44,\"retry\":55,\"expire\":66}," +
				"\"ns\":[{\"ttl\":300, \"host\":\"ns1.example.net.\"},{\"ttl\":300, \"host\":\"ns2.example.net.\"}]}",
		},
		{"sub.*",
			"{\"txt\":[{\"ttl\":300, \"text\":\"this is not a wildcard\"}]}",
		},
		{"host1",
			"{\"a\":[{\"ttl\":300, \"ip\":\"5.5.5.5\"}]}",
		},
		{"subdel",
			"{\"ns\":[{\"ttl\":300, \"host\":\"ns1.subdel.example.net.\"},{\"ttl\":300, \"host\":\"ns2.subdel.example.net.\"}]}",
		},
		{"*",
			"{\"txt\":[{\"ttl\":300, \"text\":\"this is a wildcard\"}]," +
				"\"mx\":[{\"ttl\":300, \"host\":\"host1.example.net.\",\"preference\": 10}]}",
		},
		{"_ssh._tcp.host1",
			"{\"srv\":[{\"ttl\":300, \"target\":\"tcp.example.com.\",\"port\":123,\"priority\":10,\"weight\":100}]}",
		},
		{"_ssh._tcp.host2",
			"{\"srv\":[{\"ttl\":300, \"target\":\"tcp.example.com.\",\"port\":123,\"priority\":10,\"weight\":100}]}",
		},
	},
	// Example.test - CNAME chain tests
	{
		{"@",
			"{\"soa\":{\"ttl\":300, \"minttl\":100, \"mbox\":\"hostmaster.example.test.\",\"ns\":\"ns1.example.test.\",\"refresh\":44,\"retry\":55,\"expire\":66}," +
				"\"ns\":[{\"ttl\":300, \"host\":\"ns1.example.test.\"},{\"ttl\":300, \"host\":\"ns2.example.test.\"}]}",
		},
		// CNAME chain: www -> web -> server -> IP
		{"www",
			"{\"cname\":[{\"ttl\":300, \"host\":\"web.example.test.\"}]}",
		},
		{"web",
			"{\"cname\":[{\"ttl\":300, \"host\":\"server.example.test.\"}]}",
		},
		{"server",
			"{\"a\":[{\"ttl\":300, \"ip\":\"10.0.0.1\"}]," +
				"\"aaaa\":[{\"ttl\":300, \"ip\":\"2001:db8::1\"}]}",
		},
		// CNAME chain with 4 levels: app -> service -> backend -> final -> IP
		{"app",
			"{\"cname\":[{\"ttl\":300, \"host\":\"service.example.test.\"}]}",
		},
		{"service",
			"{\"cname\":[{\"ttl\":300, \"host\":\"backend.example.test.\"}]}",
		},
		{"backend",
			"{\"cname\":[{\"ttl\":300, \"host\":\"final.example.test.\"}]}",
		},
		{"final",
			"{\"a\":[{\"ttl\":300, \"ip\":\"10.0.0.2\"}]}",
		},
		// CNAME pointing to a CNAME that has both CNAME and A (edge case)
		{"alias",
			"{\"cname\":[{\"ttl\":300, \"host\":\"dual.example.test.\"}]}",
		},
		{"dual",
			"{\"cname\":[{\"ttl\":300, \"host\":\"target.example.test.\"}]," +
				"\"a\":[{\"ttl\":300, \"ip\":\"10.0.0.99\"}]}",
		},
		{"target",
			"{\"a\":[{\"ttl\":300, \"ip\":\"10.0.0.3\"}]}",
		},
		// CNAME chain that stops mid-chain (points to non-existent)
		{"broken",
			"{\"cname\":[{\"ttl\":300, \"host\":\"missing.example.test.\"}]}",
		},
		// CNAME pointing outside the zone
		{"external",
			"{\"cname\":[{\"ttl\":300, \"host\":\"www.example.org.\"}]}",
		},
		// Direct A record (no CNAME)
		{"direct",
			"{\"a\":[{\"ttl\":300, \"ip\":\"10.0.0.100\"}]}",
		},
	},
}

var testCases = [][]test.Case{
	// basic tests
	{
		// A Test
		{
			Qname: "x.example.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("x.example.com. 300 IN A 1.2.3.4"),
				test.A("x.example.com. 300 IN A 5.6.7.8"),
			},
		},
		// AAAA Test
		{
			Qname: "x.example.com.", Qtype: dns.TypeAAAA,
			Answer: []dns.RR{
				test.AAAA("x.example.com. 300 IN AAAA ::1"),
			},
		},
		// TXT Test
		{
			Qname: "x.example.com.", Qtype: dns.TypeTXT,
			Answer: []dns.RR{
				test.TXT("x.example.com. 300 IN TXT bar"),
				test.TXT("x.example.com. 300 IN TXT foo"),
			},
		},
		// CNAME Test
		{
			Qname: "y.example.com.", Qtype: dns.TypeCNAME,
			Answer: []dns.RR{
				test.CNAME("y.example.com. 300 IN CNAME x.example.com."),
			},
		},
		// CNAME resolution - A query on CNAME should return CNAME + A records
		// Note: SortAndCheck sorts by name, so A records (x.) come before CNAME (y.)
		{
			Qname: "y.example.com.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("x.example.com. 300 IN A 1.2.3.4"),
				test.A("x.example.com. 300 IN A 5.6.7.8"),
				test.CNAME("y.example.com. 300 IN CNAME x.example.com."),
			},
		},
		// CNAME resolution - AAAA query on CNAME should return CNAME + AAAA records
		// Note: SortAndCheck sorts by name, so AAAA record (x.) comes before CNAME (y.)
		{
			Qname: "y.example.com.", Qtype: dns.TypeAAAA,
			Answer: []dns.RR{
				test.AAAA("x.example.com. 300 IN AAAA ::1"),
				test.CNAME("y.example.com. 300 IN CNAME x.example.com."),
			},
		},
		// NS Test
		{
			Qname: "x.example.com.", Qtype: dns.TypeNS,
			Answer: []dns.RR{
				test.NS("x.example.com. 300 IN NS ns1.example.com."),
				test.NS("x.example.com. 300 IN NS ns2.example.com."),
			},
			Extra: []dns.RR{
				test.A("ns1.example.com. 300 IN A 2.2.2.2"),
				test.A("ns2.example.com. 300 IN A 3.3.3.3"),
			},
		},
		// MX Test
		{
			Qname: "x.example.com.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("x.example.com. 300 IN MX 10 mx1.example.com."),
				test.MX("x.example.com. 300 IN MX 10 mx2.example.com."),
			},
		},
		// SRV Test
		{
			Qname: "_sip._tcp.example.com.", Qtype: dns.TypeSRV,
			Answer: []dns.RR{
				test.SRV("_sip._tcp.example.com. 300 IN SRV 10 100 555 sip.example.com."),
			},
			Extra: []dns.RR{
				test.A("sip.example.com. 300 IN A 7.7.7.7"),
				test.AAAA("sip.example.com 300 IN AAAA ::1"),
			},
		},
		// NXDOMAIN Test
		{
			Qname: "notexists.example.com.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
		},
		// SOA Test
		{
			Qname: "example.com.", Qtype: dns.TypeSOA,
			Answer: []dns.RR{
				test.SOA("example.com. 300 IN SOA ns1.example.com. hostmaster.example.com. 1460498836 44 55 66 100"),
			},
		},
	},
	// Wildcard Tests
	{
		{
			Qname: "host3.example.net.", Qtype: dns.TypeMX,
			Answer: []dns.RR{
				test.MX("host3.example.net. 300 IN MX 10 host1.example.net."),
			},
			Extra: []dns.RR{
				test.A("host1.example.net. 300 IN A 5.5.5.5"),
			},
		},
		{
			Qname: "host3.example.net.", Qtype: dns.TypeA,
		},
		{
			Qname: "foo.bar.example.net.", Qtype: dns.TypeTXT,
			Answer: []dns.RR{
				test.TXT("foo.bar.example.net. 300 IN TXT \"this is a wildcard\""),
			},
		},
		{
			Qname: "host1.example.net.", Qtype: dns.TypeMX,
		},
		{
			Qname: "sub.*.example.net.", Qtype: dns.TypeMX,
		},
		{
			Qname: "host.subdel.example.net.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
		},
		{
			Qname: "ghost.*.example.net.", Qtype: dns.TypeMX,
			Rcode: dns.RcodeNameError,
		},
		{
			Qname: "f.h.g.f.t.r.e.example.net.", Qtype: dns.TypeTXT,
			Answer: []dns.RR{
				test.TXT("f.h.g.f.t.r.e.example.net. 300 IN TXT \"this is a wildcard\""),
			},
		},
	},
	// CNAME chain tests
	{
		// Simple 2-level CNAME chain (www -> web -> server)
		{
			Qname: "www.example.test.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("server.example.test. 300 IN A 10.0.0.1"),
				test.CNAME("web.example.test. 300 IN CNAME server.example.test."),
				test.CNAME("www.example.test. 300 IN CNAME web.example.test."),
			},
		},
		// CNAME chain with AAAA query
		{
			Qname: "www.example.test.", Qtype: dns.TypeAAAA,
			Answer: []dns.RR{
				test.AAAA("server.example.test. 300 IN AAAA 2001:db8::1"),
				test.CNAME("web.example.test. 300 IN CNAME server.example.test."),
				test.CNAME("www.example.test. 300 IN CNAME web.example.test."),
			},
		},
		// 4-level CNAME chain (app -> service -> backend -> final)
		{
			Qname: "app.example.test.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.CNAME("app.example.test. 300 IN CNAME service.example.test."),
				test.CNAME("backend.example.test. 300 IN CNAME final.example.test."),
				test.A("final.example.test. 300 IN A 10.0.0.2"),
				test.CNAME("service.example.test. 300 IN CNAME backend.example.test."),
			},
		},
		// CNAME pointing to record with both CNAME and A (should follow CNAME)
		{
			Qname: "alias.example.test.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.CNAME("alias.example.test. 300 IN CNAME dual.example.test."),
				test.CNAME("dual.example.test. 300 IN CNAME target.example.test."),
				test.A("target.example.test. 300 IN A 10.0.0.3"),
			},
		},
		// CNAME pointing to non-existent record (should return just CNAME)
		{
			Qname: "broken.example.test.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.CNAME("broken.example.test. 300 IN CNAME missing.example.test."),
			},
		},
		// CNAME pointing outside zone (should return just CNAME)
		{
			Qname: "external.example.test.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.CNAME("external.example.test. 300 IN CNAME www.example.org."),
			},
		},
		// Direct A record (no CNAME chain)
		{
			Qname: "direct.example.test.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("direct.example.test. 300 IN A 10.0.0.100"),
			},
		},
		// Query for CNAME type on a chain should only return first CNAME
		{
			Qname: "www.example.test.", Qtype: dns.TypeCNAME,
			Answer: []dns.RR{
				test.CNAME("www.example.test. 300 IN CNAME web.example.test."),
			},
		},
		// Middle of chain accessed directly
		{
			Qname: "web.example.test.", Qtype: dns.TypeA,
			Answer: []dns.RR{
				test.A("server.example.test. 300 IN A 10.0.0.1"),
				test.CNAME("web.example.test. 300 IN CNAME server.example.test."),
			},
		},
	},
}

func newRedisPlugin() *Redis {
	ctxt = context.TODO()

	redis := new(Redis)
	redis.keyPrefix = ""
	redis.keySuffix = ""
	redis.Ttl = 300
	redis.redisAddress = "localhost:6379"
	redis.redisPassword = ""
	redis.Connect()
	redis.LoadZones()
	return redis
	/*
		return &Redis {
			keyPrefix: "",
			keySuffix:"",
			redisc: client,
			Ttl: 300,
		}	redis := new(Redis)
	*/
}

// TestAnswer is an integration test which requires a local Redis instance. The test
// expects an instance on localhost:6379 configured without authentication.
func TestAnswer(t *testing.T) {
	r := newRedisPlugin()
	conn := r.Pool.Get()
	defer conn.Close()

	for i, zone := range zones {
		conn.Do("EVAL", "return redis.call('del', unpack(redis.call('keys', ARGV[1])))", 0, r.keyPrefix+zone+r.keySuffix)
		for _, cmd := range lookupEntries[i] {
			err := r.save(zone, cmd[0], cmd[1])
			if err != nil {
				t.Error("error in redis", err)
				t.Fail()
			}
		}
		for _, tc := range testCases[i] {
			m := tc.Msg()

			rec := dnstest.NewRecorder(&test.ResponseWriter{})
			r.ServeDNS(ctxt, rec, m)

			resp := rec.Msg

			// TODO(arash): this shouldn't happen, check plugin's empty response
			if resp == nil {
				resp = new(dns.Msg)
			}
			if err := test.SortAndCheck(resp, tc); err != nil {
				t.Error(err)
			}
		}
	}
}

var ctxt context.Context

// TestCNAMEChainMaxDepth tests that CNAME chains are limited to prevent infinite loops.
func TestCNAMEChainMaxDepth(t *testing.T) {
	r := newRedisPlugin()
	conn := r.Pool.Get()
	defer conn.Close()

	zone := "chain.test."

	// Clean up
	conn.Do("EVAL", "return redis.call('del', unpack(redis.call('keys', ARGV[1])))", 0, r.keyPrefix+zone+r.keySuffix)

	// Create SOA
	r.save(zone, "@", "{\"soa\":{\"ttl\":300, \"minttl\":100, \"mbox\":\"hostmaster.chain.test.\",\"ns\":\"ns1.chain.test.\",\"refresh\":44,\"retry\":55,\"expire\":66}}")

	// Create a very long CNAME chain (20 levels, beyond the 16 max)
	for i := 0; i < 20; i++ {
		var target string
		if i == 19 {
			// Last one points to an A record
			target = "final.chain.test."
		} else {
			target = "host" + string(rune('0'+i+1)) + ".chain.test."
		}
		r.save(zone, "host"+string(rune('0'+i)), "{\"cname\":[{\"ttl\":300, \"host\":\""+target+"\"}]}")
	}

	// Create the final A record
	r.save(zone, "final", "{\"a\":[{\"ttl\":300, \"ip\":\"10.0.0.99\"}]}")

	// Reload zones
	r.LoadZones()

	// Query the first host in the chain
	m := new(dns.Msg)
	m.SetQuestion("host0.chain.test.", dns.TypeA)

	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	r.ServeDNS(ctxt, rec, m)

	resp := rec.Msg
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}

	// Should get a response with CNAMEs, but not all 20
	// The function should stop at maxCNAMEDepth (16)
	if len(resp.Answer) == 0 {
		t.Error("Expected CNAME answers, got none")
	}

	// Count CNAMEs in the response
	cnameCount := 0
	for _, rr := range resp.Answer {
		if _, ok := rr.(*dns.CNAME); ok {
			cnameCount++
		}
	}

	// Should have hit the max depth limit (16), so at most 16 CNAMEs
	if cnameCount > 16 {
		t.Errorf("Expected at most 16 CNAMEs due to max depth, got %d", cnameCount)
	}

	t.Logf("CNAME chain correctly limited to %d CNAMEs", cnameCount)
}
