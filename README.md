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
- EPS pacing, duration-bounded streams, and exact per-event sizing in YAML streams
- File splitting for plain and gzip file outputs

## Supported Log Formats

- `apache_common`
- `apache_combined`
- `apache_error`
- `rfc3164`
- `rfc5424`
- `cef` (CEF:0 over RFC5424)
- `common_log`
- `json`

### Syslog and CEF

- `rfc3164` uses a local timestamp such as `Apr  7 09:30:00`, a hostname without a domain suffix, and an alphanumeric TAG of at most 32 characters. Content is visible ASCII; only the content is truncated when needed. The body is limited to 1023 bytes, reserving one byte for the existing terminal LF so a UDP datagram is at most 1024 bytes.
- `rfc5424` uses VERSION `1`, bounded printable-ASCII header fields, and `-` for STRUCTURED-DATA. Timestamps preserve the timezone: UTC uses `Z`, while UTC+07:00 uses `+07:00`.
- `cef` places a CEF:0 payload in the MSG of an RFC5424 envelope. It uses a stable synthetic identity (`DeviceVendor=isc4`, `DeviceProduct=flog`, event class `1001`) and the project version as DeviceVersion. It represents a network connection allowed event with IPv4 source/destination, a random source port, destination port `443`, protocol `TCP`, and action `allowed`.

CEF always uses syslog **local4.warning**, PRI **164** (`20 * 8 + 4`). The CEF Severity header is independently fixed at **5** (Medium); it is not derived from syslog severity. Extensions appear in this order: `src`, `dst`, `spt`, `dpt`, `proto`, `act`, `msg`. CEF escaping keeps logical CR/LF inside values as literal `\r`/`\n` sequences, so each generated event occupies one physical line.

```bash
./flog -f cef -t udp --target 192.168.1.10:514 -n 100
./flog -f cef -t tcp --target 192.168.1.10:515 -n 100
```

All outputs retain the existing terminal LF. It is output/framing behavior, separate from the syslog/CEF message grammar. TCP uses newline framing, not RFC6587 octet-counted framing. Raw CEF, selectable envelopes/CEF versions, and configurable CEF fields are not supported.

Specifications: [RFC3164](https://www.rfc-editor.org/rfc/rfc3164), [RFC5424](https://www.rfc-editor.org/rfc/rfc5424), [OpenText CEF Implementation Standard v27](https://docs.microfocus.com/doc/2097/26.1/siemcefimplementationstandard).

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

The single-stream CLI generates one format to one output type and one destination. Use separate processes or YAML multi-stream configuration for concurrent destinations.

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
    eps: 25
    event_size: 1024

  - name: apache-tcp
    format: apache_combined
    type: tcp
    target: 192.168.1.10:515
    loop: true
    eps: 10
    duration: 30s
```

Run it with:

```bash
./flog --config flog.yaml
```

The required fields are `name`, `format`, `type`, and `target`. The optional fields are `number`, `loop`, `delay`, `eps`, `duration`, and `event_size`. `type` must be `tcp` or `udp`; `target` must be a valid `host:port` destination. `eps` is a positive integer target rate in events per second and cannot be combined with `delay`, including `delay: 0s`. `duration` is a positive Go duration such as `10s`, `30s`, or `1m`; it starts after the stream writer or network connection has been initialized.

When both `number` and `loop` are omitted, the existing default finite count (`1000`) is used. When `duration` is present and both `number` and `loop` are omitted, the stream runs continuously until its duration expires instead. `number` and `loop: true` cannot be specified together. A stream stops when its applicable count or duration limit is reached first. The first EPS event is scheduled immediately, and later events remain anchored to the planned EPS timeline rather than accumulating generation or write time. In EPS mode, the generated log timestamp is that planned event time. The complete file is validated before any stream starts.

Duration expiry is normal stream completion and does not stop other healthy streams. If one stream has a connection or write error, isc4-flog stops the remaining streams and reports the failing stream name. Ctrl+C stops all active streams cleanly. TCP connections are still one-per-stream and reused; each UDP stream uses its own connected socket and emits one log per datagram.

`bytes` remains the legacy single-stream total-output byte target. `event_size` is different: it sets the exact application bytes for each YAML stream event, including the terminal LF written after the serialized record. It does not change number, loop, EPS, duration, or legacy `bytes` behavior.

`event_size` is validated before any configured stream starts. The minimum is format-specific: Apache common/common-log `95`, Apache combined `368`, Apache error `108`, RFC3164 `64`, RFC5424 `105`, CEF `225`, and JSON `329` bytes. RFC3164 has a hard maximum of `1024` bytes including the LF. Phase-one CEF resizes only the existing `msg` extension, so its supported range is `225..1225` bytes; the `msg` value remains within the OpenText CEF Implementation Standard Version 27 limit of 1023 bytes.

isc4-flog applies a 1 MiB per-event safety limit for TCP and a 65507-byte UDP application-payload limit. These are isc4-flog product/transport policies, not RFC5424 message-size limits. RFC5424 has no general protocol-wide maximum. UDP path MTU and fragmentation often make smaller messages preferable; RFC5426 interoperability recommendations are receiver-support targets, not sender maxima. Receiver limits such as rsyslog `maxMessageSize` are receiver configuration and can still truncate or reject larger messages.

CEF uses the same configuration fields. For example, save this as `flog.yaml` and run `./flog --config flog.yaml`:

```yaml
streams:
  - name: cef-udp
    format: cef
    type: udp
    target: 192.168.1.10:514
    number: 100
  - name: cef-tcp
    format: cef
    type: tcp
    target: 192.168.1.10:515
    number: 100
```

## Rsyslog Examples

These minimal receiver configurations write parsed message bodies (`%msg%`) to a file. Adjust the port and output path for your environment.

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

### Manual CEF acceptance with AMA / Microsoft Sentinel

The acceptance path is `isc4-flog -> Rsyslog -> AMA -> CommonSecurityLog`. Configure the [CEF via AMA connector and DCR](https://learn.microsoft.com/en-us/azure/sentinel/connect-cef-syslog-ama) to collect `local4` at Warning or a more inclusive minimum severity.

The file-only rulesets above do not automatically invoke AMA's default-ruleset forwarding rules. On the AMA forwarder, use inputs in the default ruleset and retain the connector-installed forwarding configuration. For example (reuse existing modules/listeners instead of loading or binding them twice):

```conf
module(load="imudp")
module(load="imtcp")
input(type="imudp" port="514")
input(type="imtcp" port="515" supportOctetCountedFraming="off")
```

Send the single-stream CEF examples above, then query the workspace:

```kusto
CommonSecurityLog
| where TimeGenerated > ago(1h)
| where DeviceVendor == "isc4" and DeviceProduct == "flog"
| where DeviceEventClassID == "1001"
| project TimeGenerated, DeviceVersion, Activity, LogSeverity,
          SourceIP, DestinationIP, SourcePort, DestinationPort,
          Protocol, DeviceAction, Message
```

Check the fields against the [Microsoft CEF mapping](https://learn.microsoft.com/en-us/azure/sentinel/cef-name-mapping). In particular, protocol should be `TCP`, destination port `443`, action `allowed`, and CEF severity Medium (`5`). These describe the synthetic event even when its delivery uses UDP.

For local inspection, Rsyslog parses the syslog header and exposes CEF in `%msg%`; `%rawmsg%` is useful for diagnostics but is not a guarantee of byte-identical network data. Capture actual packets, including the UDP terminal LF, with:

```bash
sudo tcpdump -i any -nn -s 0 -X 'udp dst port 514 or tcp dst port 515'
```

Verify one LF-terminated CEF event per UDP datagram and newline framing on TCP. Rsyslog parsing and Azure ingestion are manual acceptance checks; automated tests do not require either service.

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
