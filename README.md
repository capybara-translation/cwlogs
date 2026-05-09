# What's this?

cwlogs prints Log Events for AWS CloudWatch. 

# Usage

```
$ ./cwlogs [--utc] [--profile <name>] [--region <region>] [--no-normalize-newlines] <log_group_name> <start> <end>
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
- `--no-normalize-newlines` — disable output normalization. By default cwlogs (1) converts `\r\n` and standalone `\r` inside each log message to `\n`, and (2) appends a trailing `\n` to any message that doesn't already end with one, so each event renders on its own line and downstream tools (`grep`, `awk`, etc.) work consistently. Pass this flag to emit the original bytes from CloudWatch Logs unchanged (e.g. when piping to a binary-aware consumer).

Dates are interpreted in the system's local timezone by default.

Note: all flags must appear *before* the positional arguments. Go's standard `flag` package stops parsing flags at the first positional argument, so `./cwlogs <log_group> --utc 20241001 20241031` will be rejected.

For example:

```
$ ./cwlogs /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --utc /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --profile staging /aws/lambda/my-function 2024-10-01 2024-10-31
$ ./cwlogs --region us-east-1 --profile staging /aws/lambda/my-function 2024-10-01T09:00:00 2024-10-01T18:00:00
```
