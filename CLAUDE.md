# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **coredns-redis**, a CoreDNS plugin that adds Redis backend support for authoritative DNS zone data. Unlike the main CoreDNS project, this is a custom plugin that integrates into CoreDNS via the plugin system. The key feature is **dynamic zone reloading** - zones added to Redis are detected within 10 minutes (or immediately when DBSIZE changes) without requiring a DNS server restart.

## Build Commands

### Building CoreDNS with the Plugin (Docker)

**Production build:**
```bash
docker build -f build/Dockerfile -t coredns-redis .
```

**Development build (uses local plugin source):**
```bash
docker build -f build/Dockerfile.dev -t coredns-redis:dev .
```

The build process:
1. Clones the official CoreDNS repository
2. Replaces `plugin.cfg` with custom version that includes this plugin
3. Runs `go generate` to regenerate plugin loading code
4. Runs `make` to build CoreDNS binary with the plugin integrated

**CRITICAL:** The plugin must be registered in `build/plugin.cfg` at the correct position (right after etcd):
```
secondary:secondary
redis:github.com/marcobox/coredns-redis
etcd:etcd
```

### Testing

**Run all tests:**
```bash
go test -v
```

**Run specific test:**
```bash
go test -v -run TestLookup
```

**Run benchmarks:**
```bash
go test -bench=. -benchmem
```

**IMPORTANT:** Tests require a live Redis instance running on `localhost:6379`. The test suite:
- Populates test zones (example.com, example.net, example.test)
- Tests all DNS record types (A, AAAA, CNAME, TXT, NS, MX, SRV, SOA, CAA)
- Tests wildcard resolution and NXDOMAIN responses
- Cleans up test data automatically

### Running the Plugin

```bash
docker run -p 53:53/udp -p 53:53 \
  -e COREDNS_REDIS_URL=redis:6379 \
  -e COREDNS_REDIS_PASSWORD=secret \
  -e COREDNS_REDIS_PREFIX="hostsync/coredns:" \
  -e COREDNS_REDIS_DNS_TTL=60 \
  coredns-redis
```

## Architecture

### Plugin Integration Model

This plugin integrates with CoreDNS using the Caddy plugin framework:

1. **Registration** ([setup.go:7](setup.go#L7)): Plugin registers itself via `caddy.RegisterPlugin("redis", ...)`
2. **Initialization** ([setup.go](setup.go)): The `setup()` function is called during CoreDNS startup
3. **Configuration Parsing** ([setup.go](setup.go)): `redisParse()` reads Corefile directives
4. **Request Handling** ([handler.go:16](handler.go#L16)): Implements `plugin.Handler` interface with `ServeDNS()` method

### DNS Request Processing Flow

When a DNS query arrives ([handler.go](handler.go)):

1. **Zone Refresh Check**: Determines if zones need reloading (every 10 minutes OR when Redis DBSIZE changes)
2. **Zone Matching**: Finds which configured zone matches the query
3. **Location Finding**: Resolves the domain name to a Redis hash field (handles wildcards)
4. **Record Retrieval**: Fetches JSON-encoded record from Redis
5. **Type-Specific Processing**: Dispatches to handler for requested record type
6. **Response Construction**: Builds DNS response message

**Dynamic Reloading Logic** ([redis.go:85-91](redis.go#L85-L91)):
```go
if time.Since(redis.LastZoneUpdate) > zoneUpdateTime || redis.lastKeyCount != redis.KeyCount() {
    redis.LoadZones()
}
```

This means new zones appear automatically without DNS restart.

### Redis Data Model

**Zones as Hash Maps:**
```
redis-cli> KEYS *
1) "example.com."
2) "example.net."
```

Each zone is a Redis hash where:
- **Key**: Zone name (e.g., "example.com.")
- **Field**: Subdomain label ("@" for apex, "www", "mail", "*" for wildcard)
- **Value**: JSON-encoded DNS record

**Example Record Structure:**
```json
{
  "a": [{"ttl": 300, "ip": "1.2.3.4"}],
  "aaaa": [{"ttl": 300, "ip": "::1"}],
  "txt": [{"ttl": 300, "text": "foo"}],
  "mx": [{"ttl": 300, "host": "mx1.example.com.", "preference": 10}],
  "srv": [{"ttl": 300, "target": "sip.example.com.", "port": 555, "priority": 10, "weight": 100}],
  "cname": [{"ttl": 300, "host": "x.example.com."}],
  "ns": [{"ttl": 300, "host": "ns1.example.com."}],
  "soa": {"ttl": 300, "minttl": 100, "mbox": "hostmaster.example.com.", "ns": "ns1.example.com.", "refresh": 44, "retry": 55, "expire": 66},
  "caa": [{"flag": 0, "tag": "issue", "value": "letsencrypt.org"}]
}
```

**Wildcard Support:**
- Use "*" as the hash field for wildcard records
- Use "sub.*" for records that should NOT be matched by wildcards
- Wildcard resolution follows DNS RFC standards with source of synthesis tracking

### Zone Discovery and Loading

**LoadZones()** ([redis.go:134](redis.go#L134)):
- Uses Redis SCAN with cursor-based iteration (batch size: 1000)
- Discovers all zone keys in the database
- For each zone, calls HKEYS to get all subdomain labels
- Handles SCAN duplicates via map deduplication

**Connection Pooling** ([redis.go:92](redis.go#L92)):
- Uses `github.com/gomodule/redigo` connection pool
- Configurable connect_timeout and read_timeout
- Supports password authentication
- Supports key prefix/suffix for namespacing

## Critical Files

### Plugin Source
- [redis.go](redis.go) - Core plugin logic, Redis operations, connection pooling, zone loading (622 lines)
- [handler.go](handler.go) - DNS request handler, zone refresh logic, record type dispatching
- [setup.go](setup.go) - Plugin registration, Corefile parsing, initialization
- [types.go](types.go) - DNS record data structures (A, AAAA, CNAME, TXT, NS, MX, SRV, SOA, CAA)

### Build Configuration
- [build/plugin.cfg](build/plugin.cfg) - **CRITICAL**: Registers plugin in CoreDNS, must be positioned correctly
- [build/Dockerfile](build/Dockerfile) - Production multi-stage build
- [build/Dockerfile.dev](build/Dockerfile.dev) - Development build with local source
- [build/entrypoint.sh](build/entrypoint.sh) - Container startup script, selects Corefile based on env vars
- [build/Corefile](build/Corefile), [build/Corefile.base](build/Corefile.base), [build/Corefile.forward](build/Corefile.forward) - DNS server configurations

### Tests
- [lookup_test.go](lookup_test.go) - Integration tests for all DNS record types, requires local Redis
- [benchmark_test.go](benchmark_test.go) - Performance benchmarks for cache hit/miss scenarios

### CI/CD
- [.github/workflows/build-push.yml](.github/workflows/build-push.yml) - GitHub Actions workflow, builds and pushes to Docker Hub + GHCR

## Configuration Reference

### Corefile Syntax
```
. {
    redis {
        address localhost:6379
        password foobared
        connect_timeout 100
        read_timeout 100
        ttl 360
        prefix _dns:
        suffix .zone
    }
}
```

### Environment Variables
- `COREDNS_REDIS_URL` - Redis server address (default: localhost:6379)
- `COREDNS_REDIS_PASSWORD` - Redis authentication password
- `COREDNS_REDIS_PREFIX` - Prefix for all Redis keys (e.g., "hostsync/coredns:")
- `COREDNS_REDIS_DNS_TTL` - Default TTL for DNS records (default: 300)
- `COREDNS_FORWARD` - If set, uses Corefile.forward with upstream forwarding

## Development Notes

### Adding New DNS Record Types

1. Add record struct to [types.go](types.go) in the `Record` struct
2. Implement parsing logic in [redis.go](redis.go) within `ServeDNS()`
3. Add test cases to [lookup_test.go](lookup_test.go)
4. Update README.md with JSON format example

### Modifying Zone Reload Logic

The zone reload mechanism is in [handler.go](handler.go) at the start of `ServeDNS()`. Current behavior:
- Checks every DNS request (minimal overhead)
- Reloads if 10+ minutes elapsed OR Redis DBSIZE changed
- Uses `redis.lastKeyCount` to track DBSIZE changes

To change reload frequency, modify `zoneUpdateTime` constant ([handler.go:15](handler.go#L15)).

### Plugin Load Order

Plugin order matters in CoreDNS. This plugin should be positioned after `secondary` and before `etcd` in [build/plugin.cfg](build/plugin.cfg). The order determines request processing sequence - plugins earlier in the chain get first chance to handle requests.

### Testing with Real Redis Data

To manually test the plugin:

1. Start Redis: `docker run -p 6379:6379 redis:alpine`
2. Add a test zone:
```bash
redis-cli HSET "test.local." "@" '{"soa":{"ttl":300,"minttl":100,"mbox":"admin.test.local.","ns":"ns1.test.local.","refresh":3600,"retry":600,"expire":86400},"a":[{"ttl":300,"ip":"127.0.0.1"}]}'
```
3. Build and run CoreDNS with the plugin
4. Query: `dig @localhost test.local A`

## Known Limitations

- **Reverse zones not supported** - PTR records for reverse DNS lookups are not implemented
- **No proxy support** - Cannot act as a recursive resolver, only authoritative
- **Zone refresh is periodic** - 10-minute maximum delay for detecting new zones (unless DBSIZE changes)
- **No incremental updates** - Zone reloads are full scans, not delta updates

## Module Information

- **Module path**: `github.com/marcobox/coredns-redis`
- **Go version**: 1.25.5
- **CoreDNS version**: v1.13.1
- **Redis client**: `github.com/gomodule/redigo` v1.9.3
