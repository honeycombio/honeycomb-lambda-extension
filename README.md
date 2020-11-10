# honeycomb-lambda-extension

The honeycomb-lambda-extension allows you to send messages from your lambda
function to Honeycomb by just writing JSON to stdout. The Honeycomb Lambda
Extension will receive the messages your function sends to stdout and forward
them to Honeycomb as events.

The extension will also send platform events such as invocation start and
shutdown events.

## Usage

To use the honeycomb-lambda-extension with a lambda function, it must be configured as a layer. This can be done with the aws CLI tool:

```
$ aws lambda update-code-configuration --function-name MyLambdaFunction --layers <extension-arn>
```

Where `<extension-arn>` is the ARN obtained after deploying the extension.

## Building & Deploying

```
$ GOOS=linux GOARCH=amd64 go build -o bin/extensions/honeycomb-lambda-extension main.go
$ cd bin
$ zip -r extension.zip extensions/
$ aws lambda publish-layer-version --layer-name honeycomb-lambda-extension \
    --region us-east-1 --zip-file "fileb://extension.zip"
```

This will produce a response from AWS that includes a `LayerVersionArn`. This is the value that customers will need to add the extension as a layer to their lambda functions.

