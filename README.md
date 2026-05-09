# What's this?

cwlogs prints Log Events for AWS CloudWatch. 

# Install

## Homebrew (macOS / Linux)

```
brew install capybara-translation/tap/cwlogs
```

## go install

```
go install github.com/capybara-translation/cwlogs/cmd/cwlogs@latest
```

## Pre-built binaries

Download the archive for your platform from the [Releases page](https://github.com/capybara-translation/cwlogs/releases) and extract the `cwlogs` binary into a directory on your `PATH`.

## Build from source

```
git clone https://github.com/capybara-translation/cwlogs.git
cd cwlogs
go build -ldflags "-s -w -X main.version=$(git describe --tags --always --dirty)" -o cwlogs ./cmd/cwlogs
```

The version reported by `cwlogs --version` depends on the build path:

| Build path | `cwlogs --version` |
|---|---|
| Homebrew / GitHub Releases (built by GoReleaser) | the released tag, e.g. `cwlogs v1.2.3` |
| `go install ...@vX.Y.Z` | `cwlogs vX.Y.Z` |
| `go install ...@latest` from a non-tagged commit | `cwlogs dev` (pseudo versions are intentionally hidden) |
| Manual `go build` with the `ldflags` example above | whatever `git describe` resolves to |
| Plain `go build` without `ldflags` | `cwlogs dev` |

# Usage

```
$ ./cwlogs [--utc] [--profile <name>] [--region <region>] [--no-normalize-newlines] [--format <format>] <log_group_name> <start> <end>
```

`<start>` and `<end>` accept any of:

- `YYYYMMDD` (e.g. `20241001`) — whole day
- `YYYY-MM-DD` (e.g. `2024-10-01`) — whole day
- `YYYY-MM-DDTHH:MM:SS` (e.g. `2024-10-01T12:34:56`) — second precision

`<start>` is the first millisecond of the chosen granule; `<end>` extends to the last millisecond of its granule (so `2024-10-01` ends at `23:59:59.999` and `2024-10-01T12:34:56` ends at `12:34:56.999`). Mixing granularities between `<start>` and `<end>` is allowed. Trailing zone designators (`Z`, `+09:00`, ...) are not accepted; use `--utc` to switch from local time to UTC.

## Options

- `--utc` — interpret start/end timestamps as UTC instead of the system's local timezone (useful in CI / containers where the host timezone is UTC).
- `--profile <name>` — AWS shared config profile to use. If omitted, the SDK's default resolution applies (including the `AWS_PROFILE` environment variable).
- `--region <region>` — AWS region (e.g. `us-east-1`). When omitted, the SDK resolves the region in this order: `AWS_REGION` → `AWS_DEFAULT_REGION` → the profile's `region` setting → EC2 IMDS (when running on EC2).
- `--no-normalize-newlines` — disable output normalization. By default cwlogs (1) converts `\r\n` and standalone `\r` inside each log message to `\n`, and (2) collapses any trailing run of `\n` to a single `\n`, so each event renders on exactly one line and stray blank lines from build/install logs (which often embed `\n\n` at the end of a section) don't appear. Blank events themselves are still emitted (one blank line for `raw`, `<ts>\t\n` for `with-time`, a JSON object for `jsonl`) so timestamps and event ordering are preserved. Pass this flag to emit the original bytes from CloudWatch Logs unchanged (e.g. when piping to a binary-aware consumer).
- `--format <format>` — output format. One of:
    - `raw` (default) — message only, matching previous behavior.
    - `with-time` — `<timestamp>\t<message>` per event. Timestamp is ISO 8601 with millisecond precision (`2024-10-01T12:34:56.789Z` in UTC, `2024-10-01T21:34:56.789+09:00` otherwise) and follows `--utc`. Multiline messages keep their original newlines: only the first line is prefixed with the timestamp, so subsequent lines lack the timestamp column. If you feed the output to a TSV parser that assumes a fixed number of fields per line, prefer `--format jsonl` instead.
    - `jsonl` — one JSON object per line: `{"timestamp":"...","stream":"<log stream name>","message":"..."}`. Use with `jq` for structured filtering (e.g. `cwlogs --format jsonl ... | jq 'select(.stream | startswith("foo/"))'`).

Dates are interpreted in the system's local timezone by default.

Note: all flags must appear *before* the positional arguments. Go's standard `flag` package stops parsing flags at the first positional argument, so `./cwlogs <log_group> --utc 20241001 20241031` will be rejected.

For example:

```
$ ./cwlogs /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --utc /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --profile staging /aws/lambda/my-function 2024-10-01 2024-10-31
$ ./cwlogs --region us-east-1 --profile staging /aws/lambda/my-function 2024-10-01T09:00:00 2024-10-01T18:00:00
$ ./cwlogs --format with-time /aws/lambda/my-function 20241001 20241001
$ ./cwlogs --format jsonl /aws/lambda/my-function 2024-10-01T12:00:00 2024-10-01T13:00:00 | jq -r '.message'
```
