# isc4-flog

isc4-flog generates synthetic logs for testing log pipelines, parsers, and receivers. It supports local output and network delivery over TCP or UDP.

## Overview

isc4-flog is based on [mingrammer/flog](https://github.com/mingrammer/flog) and is maintained independently. It retains synthetic log generation and file output, adds TCP and UDP network output, and supports concurrent network streams through YAML configuration.

Use it to produce a selected log format for stdout, a file, a gzip file, or one network destination. For concurrent TCP/UDP streams, use a YAML configuration file.

## Features

- Synthetic log generation in several common formats
- Output to stdout, plain files, gzip files, TCP, or UDP
- Configurable network destination with `--target host:port`
- Optional YAML configuration for concurrent TCP and UDP streams
- Finite generation by line count or byte count
- Continuous generation with `--loop`
- File splitting for plain and gzip file outputs

## Supported Log Formats

- `apache_common`
- `apache_combined`
- `apache_error`
- `rfc3164`
- `rfc5424`
- `common_log`
- `json`

## Supported Output Types

| Type | Description |
| --- | --- |
| `stdout` | Writes generated logs to standard output. This is the default. |
| `log` | Writes generated logs to a plain file. |
| `gz` | Writes generated logs to a gzip-compressed file. |
| `tcp` | Sends newline-delimited logs through one reused TCP connection. |
| `udp` | Sends each generated log as one UDP datagram. |

`--target host:port` is required for `tcp` and `udp`. It accepts an IP address or hostname with a port.

## Installation

Build from source with Go:

```bash
git clone <your-isc4-flog-repository-url>
cd isc4-flog
go build -o flog .
```

Run the compiled binary:

```bash
./flog --help
```

On Windows, run `./flog.exe --help`.

## Quick Start

```bash
# Write 10 Apache common logs to stdout
./flog -f apache_common -n 10

# Write 100 Apache combined logs to a file
./flog -f apache_combined -t log -o access.log -n 100

# Send 100 Apache common logs over UDP
./flog -f apache_common -t udp --target 192.168.1.10:514 -n 100

# Send 100 Apache combined logs over TCP
./flog -f apache_combined -t tcp --target 192.168.1.10:515 -n 100

# Generate RFC3164 logs continuously until interrupted
./flog -f rfc3164 -t udp --target 192.168.1.10:514 --loop

# Run concurrent network streams from a YAML file
./flog --config flog.yaml
```

## Usage

```text
flog [options]
```

| Option | Description |
| --- | --- |
| `-f`, `--format` | Log format. Default: `apache_common`. |
| `-t`, `--type` | Output type. Default: `stdout`. |
| `-o`, `--output` | Output path for `log` and `gz`. Default: `generated.log`. |
| `--target` | Network destination in `host:port` form. Required for `tcp` and `udp`. |
| `--config` | YAML configuration file for concurrent TCP and UDP streams. Cannot be combined with single-stream flags. |
| `-n`, `--number` | Number of logs to generate. Default: `1000`. |
| `-b`, `--bytes` | Generate until this output size in bytes is reached. When nonzero, it is used instead of `--number`. |
| `-s`, `--sleep` | Advance each generated timestamp by this interval without waiting. A bare number is seconds. |
| `-d`, `--delay` | Wait this interval between generated logs. A bare number is seconds. |
| `-p`, `--split-by` | Split `log` and `gz` output by line count or byte size. |
| `-w`, `--overwrite` | Allow an existing `log` or `gz` output file to be overwritten. |
| `-l`, `--loop` | Generate continuously until the process is interrupted. |
| `-h`, `--help` | Show help. |
| `-v`, `--version` | Show the version. |

## Network Output

One isc4-flog process generates one format to one output type and one destination. Run separate processes when you need different formats or destinations.

### UDP

```bash
./flog -f apache_common -t udp --target 192.168.1.10:514 -n 100
```

isc4-flog opens one UDP socket. Each generated log, including its trailing newline, is sent as one UDP datagram. UDP does not guarantee delivery, and isc4-flog only returns errors reported by the operating system.

### TCP

```bash
./flog -f apache_combined -t tcp --target 192.168.1.10:515 -n 100
```

isc4-flog opens one TCP connection and reuses it for all generated logs. Messages are newline-delimited. Connection and write errors are returned; automatic reconnect is not implemented.

## Multi-stream Configuration

`--config` runs the listed TCP and UDP streams concurrently. Each stream has one format, one output type, and one target.

```yaml
streams:
  - name: apache-udp
    format: apache_common
    type: udp
    target: 192.168.1.10:514
    number: 100
    delay: 1s

  - name: apache-tcp
    format: apache_combined
    type: tcp
    target: 192.168.1.10:515
    loop: true
    delay: 500ms
```

Run it with:

```bash
./flog --config flog.yaml
```

The required fields are `name`, `format`, `type`, and `target`. The optional fields are `number`, `loop`, and `delay`. `type` must be `tcp` or `udp`; `target` must be a valid `host:port` destination. When both `number` and `loop` are omitted, the existing default finite count (`1000`) is used. `number` and `loop: true` cannot be specified together. The complete file is validated before any stream starts.

If one stream has a connection or write error, isc4-flog stops the remaining streams and reports the failing stream name. Ctrl+C stops all active streams cleanly. TCP connections are still one-per-stream and reused; each UDP stream uses its own connected socket and emits one log per datagram.

## Rsyslog Examples

These minimal receiver configurations write raw messages to a file. Adjust the port and output path for your environment.

### UDP receiver

```conf
module(load="imudp")

template(name="FlogRaw" type="string" string="%msg%\n")
ruleset(name="flog_udp") {
    action(type="omfile" file="/var/log/isc4-flog-udp.log" template="FlogRaw")
}

input(type="imudp" port="514" ruleset="flog_udp")
```

### TCP receiver

```conf
module(load="imtcp")

template(name="FlogRaw" type="string" string="%msg%\n")
ruleset(name="flog_tcp") {
    action(type="omfile" file="/var/log/isc4-flog-tcp.log" template="FlogRaw")
}

input(type="imtcp" port="515" ruleset="flog_tcp" supportOctetCountedFraming="off")
```

TCP output uses raw newline-delimited messages, not RFC6587 octet-counted framing. Setting `supportOctetCountedFraming="off"` is important for raw logs that begin with digits, such as Apache access logs beginning with an IP address; otherwise rsyslog can interpret the leading digits as an octet-counted frame length.

## Examples

Run separate processes for separate destinations when using the single-stream CLI:

```text
Process 1: apache_common -> UDP -> 192.168.1.10:514
Process 2: apache_combined -> TCP -> 192.168.1.10:515
```

```bash
# Process 1
./flog -f apache_common -t udp --target 192.168.1.10:514 -n 100

# Process 2, in another terminal
./flog -f apache_combined -t tcp --target 192.168.1.10:515 -n 100
```

## Current Limitations

- Single-stream CLI mode supports one log format, output type, and destination per process
- Multi-stream configuration supports only one `tcp` or `udp` destination per stream
- No multi-stream stdout, file, or gzip output
- No automatic TCP reconnect

## Development

```bash
# Build
go build -o flog .

# Run tests
go test ./...

# Run static checks
go vet ./...
```

## Upstream

isc4-flog is based on [mingrammer/flog](https://github.com/mingrammer/flog) and is maintained independently in this repository. The upstream project remains credited for the original log generator.

## License

isc4-flog is distributed under the [MIT License](LICENSE). The existing license retains the upstream copyright notice.
