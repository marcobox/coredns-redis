package redis

import (
	"context"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

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
		// Check for CNAME first - if present, return CNAME and follow it
		if len(record.CNAME) > 0 {
			answers, extras = redis.CNAME(qname, z, record)
			// Try to resolve the CNAME target to get the A record
			if len(answers) > 0 {
				cname := answers[0].(*dns.CNAME)
				targetLocation := redis.findLocation(cname.Target, z)
				if len(targetLocation) > 0 {
					targetRecord := redis.get(targetLocation, z)
					if targetRecord != nil {
						aAnswers, aExtras := redis.A(cname.Target, z, targetRecord)
						answers = append(answers, aAnswers...)
						extras = append(extras, aExtras...)
					}
				}
			}
		} else {
			answers, extras = redis.A(qname, z, record)
		}
	case "AAAA":
		// Check for CNAME first - if present, return CNAME and follow it
		if len(record.CNAME) > 0 {
			answers, extras = redis.CNAME(qname, z, record)
			// Try to resolve the CNAME target to get the AAAA record
			if len(answers) > 0 {
				cname := answers[0].(*dns.CNAME)
				targetLocation := redis.findLocation(cname.Target, z)
				if len(targetLocation) > 0 {
					targetRecord := redis.get(targetLocation, z)
					if targetRecord != nil {
						aaaaAnswers, aaaaExtras := redis.AAAA(cname.Target, z, targetRecord)
						answers = append(answers, aaaaAnswers...)
						extras = append(extras, aaaaExtras...)
					}
				}
			}
		} else {
			answers, extras = redis.AAAA(qname, z, record)
		}
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
