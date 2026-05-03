# What's this?

cwlogs prints Log Events for AWS CloudWatch. 

# Usage

```
$ ./cwlogs [--utc] <log_group_name> <start_date: YYYYMMDD> <end_date: YYYYMMDD> [<aws_profile>]

```

Dates are interpreted in the system's local timezone by default. Pass `--utc` to interpret them as UTC instead (useful in CI / containers where the host timezone is UTC).

Note: `--utc` must appear *before* the positional arguments. Go's standard `flag` package stops parsing flags at the first positional argument, so `./cwlogs <log_group> --utc 20241001 20241031` will be rejected.

For example:

```
$ ./cwlogs /aws/lambda/my-function 20241001 20241031
$ ./cwlogs --utc /aws/lambda/my-function 20241001 20241031
```
