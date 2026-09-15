<div align="center">

<a href="README.en.md"><img src="logo.png" alt="FlareTunnel" width="260"></a>

# FlareTunnel

**Cloudflare Workers HTTP/HTTPS proxy for egress rotation, CONNECT tunneling, and long-lived streaming.**

[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Cloudflare Workers](https://img.shields.io/badge/Cloudflare-Workers-F38020?logo=cloudflare&logoColor=white)](https://developers.cloudflare.com/workers/)
[![TLS](https://img.shields.io/badge/TLS-transport%20%2B%20MITM-2F80ED)](#tls-and-https-connect)
[![License](https://img.shields.io/badge/license-non--commercial-orange)](#license-and-responsibility)

**[Français](README.md) · English**

</div>

FlareTunnel is a local and deployable proxy that routes requests through Cloudflare Workers. It provides one HTTP proxy entry point, distributes requests across multiple Workers, enforces Basic authentication, and preserves SSE and LLM response streaming.

## Architecture

```mermaid
graph LR
    C[HTTP/HTTPS client] -->|HTTP proxy or transport TLS| P[FlareTunnel]
    P -->|Rotation| R{Worker rotator}
    R --> W1[Cloudflare Worker 1]
    R --> W2[Cloudflare Worker 2]
    R --> W3[Cloudflare Worker N]
    W1 --> T[Target website or API]
    W2 --> T
    W3 --> T
```

The `CONNECT` path works with HTTPS clients. When TLS interception is enabled, FlareTunnel establishes an explicit MITM tunnel: the client trusts the FlareTunnel public CA, and FlareTunnel creates a separate HTTPS connection to the Worker. The upstream body is forwarded without being interpreted.

## Core capabilities

- **Worker rotation** in `random` or `round-robin` mode.
- **Multiple Cloudflare accounts** with optional quota-aware distribution.
- **HTTP and HTTPS proxying** for `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`, and `CONNECT`.
- **Mandatory Basic authentication** before forwarding or tunnel establishment.
- **Optional transport TLS** on the proxy listener.
- **Optional TLS MITM** for HTTPS interception.
- **SSE and LLM streaming** with progressive chunk forwarding and no global body timeout.
- **Built-in blocklists** for reducing Worker request consumption.
- **Worker analytics and connectivity tests** per Worker and account.
- **Configuration export/import** for controlled backups.

## Requirements

- Go 1.22 or later.
- One or more Cloudflare accounts with an API token authorized to manage Workers.
- A public MITM CA certificate and private key when HTTPS interception is enabled.
- A transport server certificate and key when the listener must accept TLS directly.

## Installation and build

```bash
git clone https://github.com/johndoe237/FlareTunnel.git
cd FlareTunnel
go mod download
go build -ldflags="-s -w" -o flaretunnel .
```

The `build.sh` script builds the current platform and offers cross-compilation for Linux, Windows, and macOS.

```bash
./build.sh
```

## Cloudflare configuration

Use `config` to configure Cloudflare accounts interactively:

```bash
./flaretunnel config
```

The local configuration stores account credentials and identifiers used by management commands. Treat it as a secret: never commit it and protect its filesystem permissions.

### Create Workers

```bash
./flaretunnel create --count 5
./flaretunnel create --count 10 --distribute
./flaretunnel create --count 3 --account main
```

`--distribute` spreads creation across available accounts. `--account` limits the operation to one account.

### List and test Workers

```bash
./flaretunnel list
./flaretunnel list --verbose
./flaretunnel list --status
./flaretunnel test
./flaretunnel test --url https://httpbin.org/ip
./flaretunnel test --url https://example.com --method POST
```

`list --verbose` includes details and live status. `list --status` focuses on response times.

### Export and restore

```bash
./flaretunnel export --output my_backup.json
./flaretunnel import --input my_backup.json
./flaretunnel import --input my_backup.json --merge
```

Review backups before transferring them. They may contain Cloudflare credentials.

### Delete Workers

The bounded form is recommended:

```bash
./flaretunnel cleanup --account main --count 20 --yes
```

This deletes at most 20 existing Workers from the `main` account. For the historical full-cleanup behavior:

```bash
./flaretunnel cleanup --account main --yes
```

The bounded form requires `--account`, `--count`, and `--yes`. It does not read interactive input and never deletes more than the requested limit.

## Start the proxy

### Local HTTP mode

```bash
export AUTH_PROXY_BASIC="dXNlcjE6cGFzczE=" # Base64("user1:pass1")
./flaretunnel tunnel --verbose
```

The proxy listens on `127.0.0.1:8080` by default. Configure clients as follows:

```text
HTTP proxy  : http://127.0.0.1:8080
HTTPS proxy : http://127.0.0.1:8080
```

The client must send:

```http
Proxy-Authorization: Basic dXNlcjE6cGFzczE=
```

Missing or invalid credentials receive `407 Proxy Authentication Required` with a `Proxy-Authenticate: Basic` challenge.

### Tunnel options

```bash
./flaretunnel tunnel --verbose
./flaretunnel tunnel --workers 0,1,2 --mode random
./flaretunnel tunnel --port 9090 --blacklist blacklist.txt
./flaretunnel tunnel --upstream-proxy http://127.0.0.1:8080 --verbose
./flaretunnel tunnel --no-ssl-intercept
./flaretunnel tunnel --cache-certs
```

Available options:

| Option | Description |
| --- | --- |
| `--host` | Listen address. Default: `127.0.0.1`. |
| `--port` | Listen port. Default: `8080`. |
| `--workers` | Worker indexes, for example `0,1,2`. |
| `--mode` | `random` or `round-robin`. |
| `--blacklist` | Blocklist file to use. |
| `--upstream-proxy` | Optional upstream proxy. |
| `--upstream-verify-ssl` | Enable upstream proxy TLS verification. |
| `--cache-certs` | Keep the MITM certificate cache. |
| `--no-ssl-intercept` | Disable TLS MITM interception. |
| `--block` | Enable configured blocking. |
| `--unsafe` | Explicitly reserved for test environments. |

## TLS and HTTPS CONNECT

### TLS MITM

Set the public certificate and private key paths for the MITM CA:

```bash
export FLARETUNNEL_MITM_CA_CERT=/runtime/Flaretunnel-MITM-CA.crt
export FLARETUNNEL_MITM_CA_KEY=/runtime/Flaretunnel-MITM-CA.key
./flaretunnel tunnel --port 8080
```

Clients should install only `Flaretunnel-MITM-CA.crt` in their trust store. The private key must never be distributed to clients.

Domain certificates are cached only when `--cache-certs` is used. Protect the CA private key with `0600` permissions.

### Transport TLS listener

To protect the connection between a client and the proxy, set:

```bash
export FLARETUNNEL_TRANSPORT_CERT=/runtime/Flaretunnel-Transport.crt
export FLARETUNNEL_TRANSPORT_KEY=/runtime/Flaretunnel-Transport.key
export FLARETUNNEL_TLS_SAN="proxy.example.com 203.0.113.42"
./flaretunnel tunnel --host 0.0.0.0 --port 8080
```

`FLARETUNNEL_TLS_SAN` accepts DNS names, IPv4 addresses, and IPv6 addresses separated by spaces. `0.0.0.0` and `::` are listen addresses, not valid certificate SANs. The server certificate must contain the identities used by clients.

The transport certificate and key are server artifacts. FlareTunnel does not need the transport CA private key.

## Blocklists

Three files are included:

| File | Intended use | Expected effect |
| --- | --- | --- |
| `blacklist-minimal.txt` | Recommended for browsing | Blocks analytics, images, fonts, and source maps. |
| `blacklist.txt` | Stronger savings | Adds advertising, tracking, CSS/JS, and CDN assets. Pages may be incomplete. |
| `blacklist-aggressive.txt` | Targeted automation | Keeps mostly HTML/API traffic. Browsers may no longer work correctly. |

A blocklist reduces Worker requests but can change site rendering. Test the selected level against real traffic.

## LLM and SSE streaming

FlareTunnel does not modify LLM payloads or transform `{"stream":true}`. HTTP responses are written as they arrive; the `CONNECT` path forwards bytes directly over the hijacked TLS connection.

There is no global timeout for the body. Connection-establishment and upstream-header deadlines remain bounded. When the client disconnects, the upstream context is cancelled and the tunnel is closed.

## Python example

```python
import requests

proxy = "http://user1:pass1@127.0.0.1:8080"
proxies = {"http": proxy, "https": proxy}

response = requests.get(
    "https://httpbin.org/ip",
    proxies=proxies,
    timeout=30,
    verify=False,  # only when the test CA is not installed
)
print(response.json()["origin"])
```

In production, install the appropriate public CA in the trust store and keep TLS verification enabled.

## CLI reference

```text
Usage: flaretunnel <command> [options]

Commands:
  config     Configure Cloudflare credentials
  create     Create Workers
  list       List Workers and analytics
  test       Test Workers
  export     Export configuration
  import     Import configuration
  cleanup    Delete Workers
  tunnel     Start the local proxy
```

## Development tests

```bash
gofmt -w *.go
go test ./...
go vet ./...
go build ./...
git diff --check
```

The test suite covers proxy authentication, `CONNECT`, SSE streaming, transport TLS certificates, and bounded cleanup behavior.

## Security and limitations

Use FlareTunnel only with accounts, Workers, and destinations for which you have authorization. Follow Cloudflare’s terms and applicable law.

Do not disable TLS verification in production. Never publish CA private keys, configuration files containing tokens, or runtime certificates. Keep the MITM and transport CAs separate.

## License and responsibility

Review the repository license files and the terms of its dependencies. This project is provided for administration, testing, and research. Users are responsible for their use of the software.

## References

- [Cloudflare Workers](https://developers.cloudflare.com/workers/ "Cloudflare Workers documentation")
- [Go `crypto/tls`](https://pkg.go.dev/crypto/tls "Go TLS package")
- [HTTP CONNECT method](https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods/CONNECT "HTTP CONNECT documentation")

---

[Lire cette documentation en français](README.md)
