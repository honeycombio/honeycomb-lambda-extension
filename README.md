# honeycomb-lambda-extension

[![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/honeycomb-lambda-extension?color=success)](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md)
[![CircleCI](https://circleci.com/gh/honeycombio/honeycomb-lambda-extension.svg?style=shield)](https://circleci.com/gh/honeycombio/honeycomb-lambda-extension)

The honeycomb-lambda-extension allows you to send messages from your lambda
function to Honeycomb by just writing JSON to stdout. The Honeycomb Lambda
Extension will receive the messages your function sends to stdout and forward
them to Honeycomb as events.

Functions instrumented with OpenTelemetry can write OTLP/JSON to stdout instead,
and the extension will forward that too, with no collector to run alongside your
function. See [Sending OpenTelemetry](#sending-opentelemetry).

The extension will also send platform events such as invocation start and
shutdown events.

## Usage

To use the honeycomb-lambda-extension with a lambda function, it must be configured as a layer.
There are two variants of the extension available: one for `x86_64` architecture and one for `arm64` architecture.

You can add the extension as a layer with the AWS CLI tool:

```
$ aws lambda update-code-configuration \
  --function-name MyAwesomeFunction
  --layers "<layer version ARN>"
```

As of v11.0.0, the extension's layer version ARN follows the pattern below. ARNs for previous releases can be found in their [release notes](https://github.com/honeycombio/honeycomb-lambda-extension/releases).

```
# Layer Version ARN Pattern
arn:aws:lambda:<AWS_REGION>:702835727665:layer:honeycomb-lambda-extension-<ARCH>-<VERSION>:1
```

- `<AWS_REGION>` -
  This must match the region of the Lambda function to which you are adding the extension.
- `<ARCH>` - `x86_64` or `arm64`
  (*note*: Graviton2 `arm64` is supported in most, but not all regions.
  See [AWS Lambda Pricing](https://aws.amazon.com/lambda/pricing/) for which regions are supported.)
- `<VERSION>` -
  The release version of the extension you wish to use with periods replaced by hyphens.
  For example: v11.0.0 -> v11-0-0.
  (Dots are not allowed characters in ARNs.)

### Configuration

The extension is configurable via environment variables set for your lambda function.

- `LIBHONEY_DATASET` - The Honeycomb dataset you would like events to be sent to.
- `LIBHONEY_API_KEY` - Your Honeycomb API Key (also called Write Key).
- `LIBHONEY_API_HOST` - Optional. Mostly used for testing purposes, or to be compatible with proxies. Defaults to https://api.honeycomb.io/.
- `LOGS_API_DISABLE_PLATFORM_MSGS` - Optional. Set to "true" in order to disable "platform" messages from the logs API.
- `HONEYCOMB_DEBUG` - Optional. Set to "true" to enable debug statements and troubleshoot issues.
- `HONEYCOMB_BATCH_SEND_TIMEOUT` - Optional.
  The timeout for the complete HTTP request/response cycle for sending a batch of events Honeycomb.
  Default: 15s (15 seconds).
  Value should be given in a format parseable as a duration, such as "1m", "15s", or "750ms".
  There are other valid time units ("ns", "us"/"µs", "h"), but their use does not fit a timeout for HTTP connections made in the AWS Lambda compute environment.
  A batch send that times out has a single built-in retry; total time a lambda invocation may spend waiting is double this value.
  A very low duration may result in duplicate events, if Honeycomb data ingest is successful but slower than this timeout (rare, but possible).
- `HONEYCOMB_CONNECT_TIMEOUT` - Optional.
  This timeout setting configures how long it can take to establish a TCP connection to Honeycomb. This setting is useful if there are ever connectivity issues, as it allows an upload requests to fail faster and not wait until the much longer batch send timeout is reached.
  Default: 3s (3 seconds).
  Value should be given in a format parseable as a duration, such as "1m", "15s", or "750ms".
  There are other valid time units ("ns", "us"/"µs", "h"), but their use does not fit a timeout for HTTP connections made in the AWS Lambda compute environment.

### Sending OpenTelemetry

If your function is instrumented with OpenTelemetry, you can point the SDK's
exporter at stdout instead of running a collector alongside your function. The
extension recognizes OTLP/JSON on stdout and forwards it, so there is no
collector process, no sidecar to connect to, and no gRPC endpoint to wait for
during a cold start.

Two line formats are recognized. Traces and logs are supported in both; metrics
are not. Everything else your function writes to stdout is handled exactly as
before, so telemetry and ordinary log lines can be mixed freely.

**OTLP/JSON written directly**, one export request per line:

```json
{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"my-func"}}]},"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174","name":"handler","kind":2,"startTimeUnixNano":"1753000000000000000","endTimeUnixNano":"1753000000123000000"}]}]}]}
```

**The `otlp-stdout` exporter envelope**, which wraps a compressed export request:

```json
{"__otel_otlp_stdout":"otlp-stdout-span-exporter@0.15.0","source":"my-func","endpoint":"http://localhost:4318/v1/traces","method":"POST","content-type":"application/x-protobuf","content-encoding":"gzip","payload":"H4sIAAAA…","base64":true}
```

The `content-type` and `content-encoding` the envelope declares are honored, so
protobuf or JSON, gzip or zstd or uncompressed all work. The signal is taken from
`endpoint`. Because the payload is compressed, this format also fits far more
spans into a line before hitting the limits described below.

Every event the extension sends carries a `lambda_extension.type` field naming
the kind of telemetry it came from, translated OTLP included. That is how a query
distinguishes a span that arrived through this extension from one sent straight
to Honeycomb, in the same way Refinery annotates what passes through it.

Telemetry is routed to the dataset named by its `service.name`, matching what
would happen if it were sent to Honeycomb's OTLP endpoint directly. With a
classic API key, spans go to `LIBHONEY_DATASET` instead, while log records still
follow `service.name`.

**Keep setting `LIBHONEY_DATASET` regardless.** The extension disables itself
entirely when it is unset — you would lose platform events and ordinary log
lines too, not just OTLP. It remains the destination for everything that isn't
an OTLP payload.

#### Choosing an exporter

**Not the one called "console".** In most SDKs that name selects a
human-readable debugging exporter whose output is neither OTLP nor stable, and
the extension will not recognize it.

| SDK | Use | Avoid |
| --- | --- | --- |
| Java | `OTEL_TRACES_EXPORTER=experimental-otlp/stdout` (1.43.0+), which writes OTLP JSON straight to stdout | `logging-otlp` writes each line as `{"resource":…,"scopeSpans":…}` with no `resourceSpans` wrapper ([opentelemetry-java#6749](https://github.com/open-telemetry/opentelemetry-java/issues/6749)) and routes through `java.util.logging`, whose default formatter prefixes a timestamp and `INFO:`. `console` is a human-readable summary. |
| Node.js | [`@dev7a/otlp-stdout-span-exporter`](https://www.npmjs.com/package/@dev7a/otlp-stdout-span-exporter), which emits the envelope above | `ConsoleSpanExporter` uses `console.dir` — Node's inspect format, not JSON, multi-line, and truncated below depth 3 |
| Python | [`otlp-stdout-span-exporter`](https://pypi.org/project/otlp-stdout-span-exporter/) | `ConsoleSpanExporter` emits the SDK's own span shape via `to_json()`, multi-line and not OTLP |
| Rust | [`otlp-stdout-span-exporter`](https://crates.io/crates/otlp-stdout-span-exporter) | — |

The `otlp-stdout` exporters are community packages from the
[serverless-otlp-forwarder](https://github.com/dev7a/serverless-otlp-forwarder)
project, not part of OpenTelemetry proper. Java's `experimental-otlp/stdout` is
upstream but, as the name says, experimental.

Whatever you use, check what it actually prints before deploying. A line the
extension doesn't recognize is not dropped — it becomes an ordinary log event,
which is the symptom to look for if spans aren't arriving.

Two constraints come from Lambda's log pipeline rather than from the extension:

- **The payload must be a single line.** Pretty-printed JSON arrives as several
  unrelated log records and cannot be reassembled. Disable pretty-printing.
- **Keep batches small.** Lambda truncates very long log lines. A truncated
  payload is no longer valid JSON, so none of its spans are recovered; it
  arrives instead as one event holding the broken text in a `record` field. A
  batch span processor with a small batch size, or a simple span processor, is
  the safer choice.

### Terraform Example

If you're using an infrastructure as code tool such as [Terraform](https://www.terraform.io/) to manage your lambda functions, you can add this extension as a layer.

```
resource "aws_lambda_function" "extensions-demo-example-lambda-python" {
        function_name = "LambdaFunctionUsingHoneycombExtension"
        s3_bucket     = "lambda-function-s3-bucket-name"
        s3_key        = "lambda-functions-are-great.zip"
        handler       = "handler.func"
        runtime       = "python3.8"
        role          = aws_iam_role.lambda_role.arn

        environment {
                variables = {
                        LIBHONEY_API_KEY = "honeycomb-api-key",
                        LIBHONEY_DATASET = "lambda-extension-test"
                        LIBHONEY_API_HOST = "api.honeycomb.io"
                }
        }

        layers = [
            "arn:aws:lambda:<AWS_REGION>:702835727665:layer:honeycomb-lambda-extension-<ARCH>-<VERSION>:1"
        ]
}
```

## Self Hosting - Building & Deploying

You can also deploy this extension as a layer in your own AWS account.

### Option 1: Publish the Honeycomb-built extension

- Download the ZIP file for your target architecture from [the GitHub release](https://github.com/honeycombio/honeycomb-lambda-extension/releases).
- Publish the layer your AWS account.

```shell
$ aws lambda publish-layer-version \
    --layer-name honeycomb-lambda-extension \
    --region <AWS_REGION> \
    --compatible-architectures <ARCH> \
    --zip-file "fileb://<path to downloaded file>"
```

### Option 2: Build and publish your own extension

From a clone of this project:

```shell
$ make zips
$ aws lambda publish-layer-version \
    --layer-name honeycomb-lambda-extension \
    --region <AWS_REGION> \
    --compatible-architectures <ARCH> \
    --zip-file "fileb://artifacts/linux/extension-<ARCH>.zip"
```

## Updating AWS regions we publish to

Use the [AWS Lambda pricing](https://aws.amazon.com/lambda/pricing/) page to get list of regions that support x86_64 and arm64.

As of 2023-02-27, follow these instructions to more readily compare region names to region ids:

- View HTML source, navigate to dropdowns, copy whole ul element for each platform and add to local file (eg regions-x86_64.txt)
- Tidy up content to only keep the region ids
- Sort the file alphabetically
  - `sort -n regions-x86_64.txt > regions-x86_64-sorted.txt`
- Perform a diff on the two files
  - `diff --ignore-matching-lines --side-by-side regions-arm64-sorted.txt regions-x86_64-sorted.txt`
- Update REGIONS_WITH_ARM (supports both x86_64 and arm64) and REGIONS_NO_ARM (only supports x86_64) in [publish.sh](./publish.sh) with derived sets
  - All regions should support x86_64 and a small subset will not support arm64
- **Note**: the source sometimes shows all regions and should not be considered a reliable way to tell whether ARM is supported; this should be a spot check with the dropdown provided.

NOTE: We need to opt-in to a new region before we can publish to it.
The [Regions and zones](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html) page shows if a region requires opt-in.

## Contributions

Features, bug fixes and other changes to the extension are gladly accepted. Please open issues or a pull request with your change. Remember to add your name to the CONTRIBUTORS file!

All contributions will be released under the Apache License 2.0.
