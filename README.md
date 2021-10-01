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

To use the honeycomb-lambda-extension with a lambda function, it must be configured as a layer. There are two versions of the extension available:
- honeycomb-lambda-extension-x86_64 (for functions running on `x86_64` architecture)
- honeycomb-lambda-extension-arm64 (for functions running on `arm64` architecture)

You can add the extension as a layer with the AWS CLI tool:

```
$ aws lambda update-code-configuration --function-name MyLambdaFunction --layers "arn:aws:lambda:<AWS_REGION>:702835727665:layer:honeycomb-lambda-extension-<ARCH>:5"
```

- `<ARCH>` --> `x86_64` or `arm64` (*note*: `arm64` is only supported in [certain regions](https://aws.amazon.com/about-aws/whats-new/2021/09/better-price-performance-aws-lambda-functions-aws-graviton2-processor/))
- `<AWS_REGION>` --> AWS region you want to deploy this in

The extension will attempt to read the following environment variables from your lambda function configuration:

- `LIBHONEY_DATASET` - The Honeycomb dataset you would like events to be sent to.
- `LIBHONEY_API_KEY` - Your Honeycomb API Key (also called Write Key).
- `LIBHONEY_API_HOST` - Optional. Mostly used for testing purposes, or to be compatible with proxies. Defaults to https://api.honeycomb.io/.
- `LOGS_API_DISABLE_PLATFORM_MSGS` - Optional. Set to "true" in order to disable "platform" messages from the logs API.
- `HONEYCOMB_DEBUG` - Optional. Set to "true" to enable debug statements and troubleshoot issues.

If you're using an infrastructure as code tool such as [Terraform](https://www.terraform.io/) to manage your lambda functions, you can add this extension as a layer:

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
            "arn:aws:lambda:<AWS_REGION>:702835727665:layer:honeycomb-lambda-extension-<ARCH>:5"
        ]
}
```

## Self Hosting - Building & Deploying

You can also deploy this extension as a layer in your own AWS account. To do that, simply build
the extension and publish it yourself. Again, with the AWS CLI tool:

```
$ mkdir -p bin/extensions
$ GOOS=linux GOARCH=<ARCH> go build -o bin/extensions/honeycomb-lambda-extension .
$ cd bin
$ zip -r extension.zip extensions
$ aws lambda publish-layer-version --layer-name honeycomb-lambda-extension \
    --region <AWS_REGION> --zip-file "fileb://extension.zip"
```

Again, substituting `<AWS_REGION>` and `<ARCH>` as appropriate.

## Contributions

Features, bug fixes and other changes to the extension are gladly accepted. Please open issues or a pull request with your change. Remember to add your name to the CONTRIBUTORS file!

All contributions will be released under the Apache License 2.0.
