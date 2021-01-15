GOOS=linux
GOARCH=amd64

build:
	mkdir -p bin/extensions
	GOOS=${GOOS} GOARCH=${GOARCH} go build -o bin/extensions/honeycomb-lambda-extension .

publish: build
	cd bin && zip -r extension.zip extensions && aws lambda publish-layer-version --layer-name honeycomb-lambda-extension --region us-east-1 --zip-file "fileb://extension.zip"

publish-dev: build
	cd bin && zip -r extension.zip extensions && aws lambda publish-layer-version --layer-name honeycomb-lambda-extension-dev --region us-east-1 --zip-file "fileb://extension.zip"
