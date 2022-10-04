# Which operating system to target for a Go build?
# Defaults to linux because the extension runs in a linux compute environment.
# Override in development if you wish to build and run on your dev host.
# Example: GOOS=darwin make build
GOOS ?= linux

# CIRCLE_TAG is generally not set unless CircleCI is running a workflow
# triggered by a git tag creation.
# If set, the value will be used for the version of the build.
# If unset, determine a reasonable version identifier for current current commit
# based on the closest vX.Y.Z tag in the branch's history. For example: v10.4.0-6-ged57c1e
# --tags :: consider all tags, not only annotated tags
# --match :: a regex to select a tag that matches our version number tag scheme
CIRCLE_TAG ?= $(shell git describe --always --tags --match "v[0-9]*" HEAD)

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
