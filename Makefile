# Which operating system to target for a Go build?
# Defaults to linux because the extension runs in a linux compute environment.
# Override in development if you wish to build and run on your dev host.
# Example: GOOS=darwin make build
GOOS ?= linux

layer_name_root = honeycomb-lambda-extension

.PHONY: test
test:
ifeq (, $(shell which gotestsum))
	@echo " ***"
	@echo "Running with standard go test because gotestsum was not found on PATH. Consider installing gotestsum for friendlier test output!"
	@echo " ***"
	go test -race ./...
else
	gotestsum --junitfile unit-tests.xml --format testname -- -race ./...
endif

build:
	@echo "\n*** Building ${layer_name_root} binaries for ${GOOS}"
	mkdir -p artifacts
	GOARCH=amd64 go build -ldflags "-X main.version=${CIRCLE_TAG}" -o artifacts/${layer_name_root}-x86_64 .
	GOARCH=arm64 go build -ldflags "-X main.version=${CIRCLE_TAG}" -o artifacts/${layer_name_root}-arm64 .

publish: build
	cd bin && zip -r extension.zip extensions && aws lambda publish-layer-version --layer-name honeycomb-lambda-extension --region us-east-1 --zip-file "fileb://extension.zip"

clean:
	rm -rf artifacts
