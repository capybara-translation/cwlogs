# What's this?

cwlogs prints Log Events for AWS CloudWatch. 

# Usage

```
$ ./cwlogs [--utc] [--profile <name>] [--region <region>] <log_group_name> <start_date: YYYYMMDD> <end_date: YYYYMMDD>
```

## Options

- `--utc` — interpret start/end dates as UTC instead of the system's local timezone (useful in CI / containers where the host timezone is UTC).
- `--profile <name>` — AWS shared config profile to use. If omitted, the SDK's default resolution applies (including the `AWS_PROFILE` environment variable).
- `--region <region>` — AWS region (e.g. `us-east-1`). When omitted, the SDK resolves the region in this order: `AWS_REGION` → `AWS_DEFAULT_REGION` → the profile's `region` setting → EC2 IMDS (when running on EC2).

Dates are interpreted in the system's local timezone by default.

Note: all flags must appear *before* the positional arguments. Go's standard `flag` package stops parsing flags at the first positional argument, so `./cwlogs <log_group> --utc 20241001 20241031` will be rejected.

For example:

```
$ ./cwlogs /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --utc /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --profile staging /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --region us-east-1 --profile staging /aws/lambda/my-function 20241001 20241031
```
