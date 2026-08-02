# fasttest

A fast, terminal-based internet speed test CLI written in Go. Measures download,
upload, and latency using a variety of backends, with a live TUI progress view
or plain JSON output for scripting.

## Features

- **Multiple backends** — Cloudflare, Fast.com, Internet Speed Test, and iperf3
- **Live TUI** — a smooth animated progress spinner powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **JSON output** — `--json` flag for machine-readable results
- **Selective tests** — run download, upload, or latency independently
- **Early termination** — the test ends automatically once throughput stabilizes, instead of always running the full duration
- **Multi-server parallelism** — spread tests across multiple servers for higher accuracy

## Backends

| Backend        | Download | Upload | Latency | Notes                                            |
| -------------- | -------- | ------ | ------- | ------------------------------------------------ |
| `cloudflare`   | HTTP GET | HTTP POST | HTTP RTT | Default. No external dependencies.               |
| `fastcom`      | HTTP GET | HTTP POST | HTTP RTT | Uses Netflix's Fast.com infrastructure.          |
| `internetspeedtest` | HTTP GET | HTTP POST | HTTP RTT | Uses the Internet Speed Test service.         |
| `iperf3`       | iperf3 `-R` | iperf3 | TCP dial | Requires the `iperf3` binary in `$PATH`. Many public servers reject uploads. |

## Installation

### From source

```sh
go install github.com/Lalaggi/fasttest@latest
```

### Build locally

```sh
go build -o fasttest .
```

> The project uses [`just`](https://github.com/casey/just). Available recipes:
> `build`, `install`, `test`, `lint`, `run`, `clean`.

## Usage

```
fasttest [flags]
```

### Flags

| Flag               | Shortcut | Default      | Description                                         |
| ------------------ | -------- | ------------ | --------------------------------------------------- |
| `--backend`        | `-b`     | `cloudflare` | Backend to use (`cloudflare`, `fastcom`, `internetspeedtest`, `iperf3`) |
| `--download-only`  |          |              | Measure download speed only                         |
| `--upload-only`    |          |              | Measure upload speed only                           |
| `--ping-only`      |          |              | Measure latency only                                |
| `--duration`       |          | `20s`        | Maximum test duration                               |
| `--servers`        | `-s`     | `5`          | Number of test servers to use                       |
| `--json`           |          |              | Output results as JSON                              |
| `--verbose`        | `-v`     |              | Show detailed progress output                       |

### Examples

Run a full test with the default backend:

```sh
fasttest
```

Use a specific backend:

```sh
fasttest -b fastcom
```

Measure latency only:

```sh
fasttest --ping-only
```

Download speed only, with JSON output and a 10-second duration:

```sh
fasttest --download-only --json --duration 10s
```

Verbose mode with detailed per-sample progress:

```sh
fasttest -v --servers 10
```

### Example output

```
Using cloudflare backend...
Initializing...
Discovering servers...
  ⟳ Download... 847.23 Mbps
  ✓ Download ✓ 902.45 Mbps
  ⟳ Upload... 45.67 Mbps
  ✓ Upload ✓ 48.12 Mbps

Download    902.45 Mbps
Upload      48.12 Mbps
Latency     12.3 ms
Server      San Francisco, US
ASN         AS13335 Cloudflare, Inc.
```

### JSON output

```json
{
  "download_mbps": 902.45,
  "upload_mbps": 48.12,
  "latency": 12300000,
  "server_ip": "1.2.3.4",
  "location": "San Francisco, US",
  "asn": "AS13335 Cloudflare, Inc."
}
```

## How it works

1. **Init** — the backend fetches client information (IP, ASN, location) via a trace
   endpoint.
2. **Server discovery** — the backend returns a list of servers sorted by latency.
3. **Measurement** — download and upload tests run in parallel goroutines against
   the selected servers. Throughput is sampled every 200 ms.
4. **Early termination** — once the test has run for at least 10 seconds and the
   instantaneous rate stays within 5% of the running average for 15 consecutive
   samples (~3 s), the test finishes early. Otherwise it runs until the configured
   duration elapses.

## Dependencies

- **Go 1.26+**
- **iperf3 binary** (only when using the `iperf3` backend)

## License

This project is licensed under the GNU Affero General Public License v3.0
(AGPL-3.0). See the [LICENSE](LICENSE) file for details.
