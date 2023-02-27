# honeycomb-lambda-extension

[![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/honeycomb-lambda-extension?color=success)](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md)
[![CircleCI](https://circleci.com/gh/honeycombio/honeycomb-lambda-extension.svg?style=shield)](https://circleci.com/gh/honeycombio/honeycomb-lambda-extension)

The honeycomb-lambda-extension allows you to send messages from your lambda
function to Honeycomb by just writing JSON to stdout. The Honeycomb Lambda
Extension will receive the messages your function sends to stdout and forward
them to Honeycomb as events.

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
