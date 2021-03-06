# honeycomb-lambda-extension

[![CircleCI](https://circleci.com/gh/honeycombio/honeycomb-lambda-extension.svg?style=shield)](https://circleci.com/gh/honeycombio/honeycomb-lambda-extension)

The honeycomb-lambda-extension allows you to send messages from your lambda
function to Honeycomb by just writing JSON to stdout. The Honeycomb Lambda
Extension will receive the messages your function sends to stdout and forward
them to Honeycomb as events.

The extension will also send platform events such as invocation start and
shutdown events.

## Usage

To use the honeycomb-lambda-extension with a lambda function, it must be configured as a layer. This can be done with the aws CLI tool:

```
$ aws lambda update-code-configuration --function-name MyLambdaFunction --layers "arn:aws:lambda:<AWS_REGION>:702835727665:layer:honeycomb-lambda-extension:2"
```

Substituting `<AWS_REGION>` for the AWS region you want to deploy this in.

The extension will attempt to read the following environment variables from your lambda function configuration:

- `LIBHONEY_DATASET` - The Honeycomb dataset you would like events to be sent to.
- `LIBHONEY_API_KEY` - Your Honeycomb API Key (also called Write Key).
- `LIBHONEY_API_HOST` - Optional. Mostly used for testing purposes, or to be compatible with proxies. Defaults to https://api.honeycomb.io/.
- `LOGS_API_DISABLE_PLATFORM_MSGS` - Optional. Set to "true" in order to disable "platform" messages from the logs API.

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
            "arn:aws:lambda:us-east-1:702835727665:layer:honeycomb-lambda-extension:2"
        ]
}
```

This example uses `us-east-1`, but as above, you may substitute this section of the arn with any AWS region.

## Self Hosting - Building & Deploying

You can also deploy this extension as a layer in your own AWS account. To do that, simply build
the extension and publish it yourself. Again, with the aws CLI tool:

```
$ mkdir -p bin/extensions
$ GOOS=linux GOARCH=amd64 go build -o bin/extensions/honeycomb-lambda-extension main.go
$ cd bin
$ zip -r extension.zip extensions
$ aws lambda publish-layer-version --layer-name honeycomb-lambda-extension \
    --region <AWS_REGION> --zip-file "fileb://extension.zip"
```

Again, substituting `<AWS_REGION>` as appropriate.

## Contributions

Features, bug fixes and other changes to libhoney are gladly accepted. Please open issues or a pull request with your change. Remember to add your name to the CONTRIBUTORS file!

All contributions will be released under the Apache License 2.0.
