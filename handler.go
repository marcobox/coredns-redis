package redis

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// resolveWithCNAME handles A/AAAA queries that may have a CNAME.
// If a CNAME exists, it returns the CNAME and attempts to resolve the target.
// It follows CNAME chains within the same zone up to a maximum depth.
// If a CNAME points outside the zone and a forward plugin is configured, it resolves the external target.
// The resolver function should be either redis.A or redis.AAAA.
func (redis *Redis) resolveWithCNAME(ctx context.Context, w dns.ResponseWriter, qname string, qtype uint16, z *Zone, record *Record, resolver func(string, *Zone, *Record) ([]dns.RR, []dns.RR)) ([]dns.RR, []dns.RR) {
	// No CNAME, return the direct A/AAAA records
	if len(record.CNAME) == 0 {
		return resolver(qname, z, record)
	}

	const maxCNAMEDepth = 16 // Prevent infinite loops
	var answers, extras []dns.RR
	currentName := qname
	currentRecord := record

	// Follow CNAME chain
	for depth := 0; depth < maxCNAMEDepth; depth++ {
		// Check if current record has a CNAME
		if len(currentRecord.CNAME) == 0 {
			// End of chain, resolve the final A/AAAA records
			targetAnswers, targetExtras := resolver(currentName, z, currentRecord)
			answers = append(answers, targetAnswers...)
			extras = append(extras, targetExtras...)
			return answers, extras
		}

		// Get CNAME record
		cnameAnswers, cnameExtras := redis.CNAME(currentName, z, currentRecord)
		if len(cnameAnswers) == 0 {
			return answers, extras
		}

		// Add CNAME to answer chain
		cname := cnameAnswers[0].(*dns.CNAME)
		answers = append(answers, cname)
		extras = append(extras, cnameExtras...)

		// Try to resolve the CNAME target within the same zone
		targetLocation := redis.findLocation(cname.Target, z)
		if targetLocation == "" {
			// CNAME points outside zone
			// If we have a forward plugin, try to resolve the external target
			if redis.Next != nil {
				externalAnswers := redis.resolveExternal(ctx, w, cname.Target, qtype)
				answers = append(answers, externalAnswers...)
			}
			return answers, extras
		}

		targetRecord := redis.get(targetLocation, z)
		if targetRecord == nil {
			return answers, extras
		}

		// Continue following the chain
		currentName = cname.Target
		currentRecord = targetRecord
	}

	// Hit max depth, return what we have
	return answers, extras
}

// ServeDNS implements the plugin.Handler interface.
func (redis *Redis) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	qname := state.Name()
	qtype := state.Type()

	if time.Since(redis.LastZoneUpdate) > zoneUpdateTime || redis.lastKeyCount != redis.KeyCount() {
		redis.LoadZones()
	}

	zone := plugin.Zones(redis.Zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(qname, redis.Next, ctx, w, r)
	}

	z := redis.load(zone)
	if z == nil {
		return redis.errorResponse(state, zone, dns.RcodeServerFailure, nil)
	}

	if qtype == "AXFR" {
		records := redis.AXFR(z)

		ch := make(chan *dns.Envelope)
		tr := new(dns.Transfer)
		tr.TsigSecret = nil

		go func(ch chan *dns.Envelope) {
			j, l := 0, 0

			for i, r := range records {
				l += dns.Len(r)
				if l > transferLength {
					ch <- &dns.Envelope{RR: records[j:i]}
					l = 0
					j = i
				}
			}
			if j < len(records) {
				ch <- &dns.Envelope{RR: records[j:]}
			}
			close(ch)
		}(ch)

		err := tr.Out(w, r, ch)
		if err != nil {
			log.Error(err)
		}
		w.Hijack()
		return dns.RcodeSuccess, nil
	}

	location := redis.findLocation(qname, z)
	if len(location) == 0 { // empty, no results
		if redis.Fall.Through(qname) {
			return plugin.NextOrFailure(qname, redis.Next, ctx, w, r)
		}
		return redis.errorResponse(state, zone, dns.RcodeNameError, nil)
	}

	var answers, extras []dns.RR

	record := redis.get(location, z)
	if record == nil {
		// Record may be nil when the redis read returns an error
		return redis.errorResponse(state, zone, dns.RcodeServerFailure, nil)
	}

	switch qtype {
	case "A":
		answers, extras = redis.resolveWithCNAME(ctx, w, qname, dns.TypeA, z, record, redis.A)
	case "AAAA":
		answers, extras = redis.resolveWithCNAME(ctx, w, qname, dns.TypeAAAA, z, record, redis.AAAA)
	case "CNAME":
		answers, extras = redis.CNAME(qname, z, record)
	case "TXT":
		answers, extras = redis.TXT(qname, z, record)
	case "NS":
		answers, extras = redis.NS(qname, z, record)
	case "MX":
		answers, extras = redis.MX(qname, z, record)
	case "SRV":
		answers, extras = redis.SRV(qname, z, record)
	case "SOA":
		answers, extras = redis.SOA(qname, z, record)
	case "CAA":
		answers, extras = redis.CAA(qname, z, record)

	default:
		return redis.errorResponse(state, zone, dns.RcodeNotImplemented, nil)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	m.Answer = append(m.Answer, answers...)
	m.Extra = append(m.Extra, extras...)

	state.SizeAndDo(m)
	m = state.Scrub(m)
	_ = w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// ResponseRecorder is a dns.ResponseWriter that captures the response message
type ResponseRecorder struct {
	dns.ResponseWriter
	msg *dns.Msg
}

// WriteMsg captures the DNS response message
func (r *ResponseRecorder) WriteMsg(msg *dns.Msg) error {
	r.msg = msg
	return nil
}

// resolveExternal queries the next plugin (typically forward) to resolve an external target
func (redis *Redis) resolveExternal(ctx context.Context, w dns.ResponseWriter, target string, qtype uint16) []dns.RR {
	if redis.Next == nil {
		return nil
	}

	// Create a new DNS query for the external target
	req := new(dns.Msg)
	req.SetQuestion(target, qtype)

	// Use ResponseRecorder to capture the response
	rec := &ResponseRecorder{ResponseWriter: w}

	// Query the next plugin (forward)
	plugin.NextOrFailure(target, redis.Next, ctx, rec, req)

	// Return the answers from the captured response
	if rec.msg != nil {
		return rec.msg.Answer
	}
	return nil
}

// Name implements the Handler interface.
func (redis *Redis) Name() string { return "redis" }

func (redis *Redis) errorResponse(state request.Request, _ string, rcode int, err error) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(state.Req, rcode)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	state.SizeAndDo(m)
	_ = state.W.WriteMsg(m)
	// Return success as the rcode to signal we have written to the client.
	return dns.RcodeSuccess, err
}
