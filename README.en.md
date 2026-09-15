# FlareTunnel

[Version française](README.md)

FlareTunnel is an HTTP/HTTPS proxy that routes requests through Cloudflare Workers. It supports Worker rotation, multiple Cloudflare accounts, proxy authentication, HTTP passthrough, the `CONNECT` method, optional TLS MITM interception, and long-lived SSE responses.

> FlareTunnel is a proxy component. The recommended production deployment uses `FlareTunnel-Manager`, which supplies the binary, certificates, and secrets at runtime.

## Architecture

```text
Client / OmniRoute
        │
        │ HTTP proxy or HTTPS transport proxy
        ▼
FlareTunnel
        │  CONNECT + optional TLS MITM
        ▼
Cloudflare Worker
        │
        ▼
Provider or target website
```

Transport TLS and MITM TLS are separate layers. Transport TLS protects the connection between the client and the proxy listener. MITM TLS presents the client with a temporary certificate for the target domain, signed by the MITM CA. FlareTunnel then creates its own HTTPS connection to the Worker.

## Features

- Random or round-robin Worker rotation.
- Multiple Cloudflare account support.
- Standard HTTP proxy and HTTPS tunneling with `CONNECT`.
- Mandatory `Proxy-Authorization: Basic` authentication.
- Optional transport TLS directly in FlareTunnel.
- Optional TLS MITM for HTTPS requests.
- Progressive forwarding of SSE bodies and long LLM responses.
- Connection and upstream-header deadlines without a global SSE-body timeout.
- Minimal, full, and aggressive blocklists.

## Local installation

```bash
git clone https://github.com/johndoe237/FlareTunnel.git
cd FlareTunnel
go build -o flaretunnel .
```

Go 1.21 or later is recommended.

## Proxy configuration

Proxy authentication is mandatory. `AUTH_PROXY_BASIC` contains the Base64 encoding of `username:password`.

```bash
export AUTH_PROXY_BASIC="dXNlcjE6cGFzczE="
```

This value represents `user1:pass1`. It must not be confused with a plaintext password.

### Local HTTP mode

Local HTTP mode is enabled when the transport TLS variables are absent.

```bash
./flaretunnel tunnel \
  --host 127.0.0.1 \
  --port 8080 \
  --mode round-robin \
  --blacklist blacklist-minimal.txt
```

### TLS MITM for `CONNECT`

To intercept HTTPS connections, FlareTunnel receives the public certificate and private key of the MITM CA through file paths. The private key must never be committed or copied into a client image.

```bash
export FLARETUNNEL_MITM_CA_CERT=/runtime/Flaretunnel-MITM-CA.crt
export FLARETUNNEL_MITM_CA_KEY=/runtime/Flaretunnel-MITM-CA.key
./flaretunnel tunnel --port 8080
```

The client must trust the public `Flaretunnel-MITM-CA.crt` certificate.

### Transport TLS listener

The listener accepts a TLS connection when all of the following variables are set:

```bash
export FLARETUNNEL_TRANSPORT_CERT=/runtime/Flaretunnel-Transport.crt
export FLARETUNNEL_TRANSPORT_KEY=/runtime/Flaretunnel-Transport.key
export FLARETUNNEL_TLS_SAN="proxy.example.com 203.0.113.42"
./flaretunnel tunnel --port 8080
```

`FLARETUNNEL_TLS_SAN` accepts DNS names, IPv4 addresses, and IPv6 addresses separated by spaces. `0.0.0.0` and `::` are listen addresses, not certificate identities, and are rejected. The server certificate must be signed by the transport CA and contain the configured SANs.

FlareTunnel does not use the transport CA private key. That key remains in the manager, which generates the ephemeral server certificate.

## Commands

| Command | Function |
| --- | --- |
| `config` | Configure Cloudflare accounts. |
| `create` | Create proxy Workers. |
| `list` | List available Workers. |
| `test` | Test Worker connectivity. |
| `tunnel` | Start the local proxy. |
| `export` | Export the configuration. |
| `import` | Import a configuration. |
| `cleanup` | Delete Workers, preferably using the bounded form. |

Examples:

```bash
./flaretunnel config
./flaretunnel create --count 5 --account main
./flaretunnel list --verbose
./flaretunnel test --url https://example.com
./flaretunnel cleanup --account main --count 5 --yes
```

The bounded `cleanup` form requires `--account`, `--count`, and `--yes`. It never deletes more Workers than the requested limit.

## Using FlareTunnel as a proxy

For a standard HTTP client:

```text
HTTP proxy  : http://proxy-user:password@127.0.0.1:8080
HTTPS proxy : http://proxy-user:password@127.0.0.1:8080
```

For a transport-TLS listener:

```text
HTTPS proxy : https://proxy-user:password@proxy.example.com:8080
```

The transport CA public certificate must be installed in the client trust store. The MITM CA is required separately to validate certificates for intercepted domains.

## Streaming and passthrough

FlareTunnel does not parse LLM bodies or transform `{"stream":true}`. The HTTP path writes each received block and calls `Flush` when supported by the server. The `CONNECT` path writes directly to the hijacked TLS connection and recreates only the HTTP framing that is required.

No global timeout cuts off an SSE body. When the client disconnects, the upstream request context is cancelled and the tunnel is closed.

## Recommended deployment

For a PaaS, VPS, or local deployment using the same Docker image, use `FlareTunnel-Manager`. The manager builds its image with the FlareTunnel binary, embeds public certificates only, receives private keys as secrets, generates the transport certificate, and launches FlareTunnel as a child process.

`omni-boot` is a separate deployment. It does not share the manager image and communicates with FlareTunnel only through the proxy protocol and the public certificates required for TLS validation.

## Security

Never disable client-side TLS validation. Do not use `rejectUnauthorized: false` or `NODE_TLS_REJECT_UNAUTHORIZED=0`. Do not publish private keys, Base64-encoded key values, ephemeral server certificates, or runtime artifacts.

The MITM and transport CAs are independent. Do not replace them or mix their private keys. Rotating either CA requires a coordinated operation with all clients that trust it.

## Development tests

```bash
go test ./...
go vet ./...
go build ./...
git diff --check
```

## License and responsibility

See the repository license files. Use this software in accordance with Cloudflare’s terms, applicable law, and the rules of the services you access.

## References

- [Cloudflare Workers](https://developers.cloudflare.com/workers/)
- [Go TLS package](https://pkg.go.dev/crypto/tls)
- [HTTP CONNECT](https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods/CONNECT)

---

[Read this documentation in French](README.md)
