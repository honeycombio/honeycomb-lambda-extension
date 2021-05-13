SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rule

.PHONY: build publish check-env clean test
GOOS=linux
GOARCH=amd64

EXTENSION = honeycomb-lambda-extension
GO_SOURCES := main.go $(wildcard *.go) go.mod go.sum

ARTIFACTS_DIR=artifacts
#: where release artifacts will be built
$(ARTIFACTS_DIR):
	@mkdir -p $@

EXTENSION_DIR=$(ARTIFACTS_DIR)/extensions
EXECUTABLE=$(EXTENSION_DIR)/$(EXTENSION)
EXTENSION_ZIP=$(ARTIFACTS_DIR)/extension.zip

#: where the contents of the extention will be gathered
$(EXTENSION_DIR):
	@mkdir -p $@

#: compile the extension executable
build: $(EXECUTABLE)
#: the extension's executable
$(EXECUTABLE): $(EXTENSION_DIR) $(GO_SOURCES)
	@echo "+++ building: $@"
	GOOS=${GOOS} GOARCH=${GOARCH} \
		go build -ldflags "-s -w" \
		-o $@ .

#: publish the extension as a lambda layer to a specified AWS_REGION
publish_aws: zip
	@:$(call check_defined, AWS_REGION, the region to which the extension will be published)
	@echo "+++ publishing $(EXTENSION) to $(AWS_REGION)"
	aws lambda publish-layer-version \
		--layer-name $(EXTENSION) \
		--region $(AWS_REGION) \
		--zip-file "fileb://$(EXTENSION_ZIP)"

#: package up the extension in a ZIP file
zip: $(EXTENSION_ZIP)
#: the extension's zipfile for publishing to Amazon
$(EXTENSION_ZIP): $(EXECUTABLE)
	@echo "+++ zipping: $@"
	cd $(ARTIFACTS_DIR) && zip -r extension.zip extensions

COVERPROFILE=cover-source.out
#: run tests with coverage report
test:
	@echo "+++ testing"
	go test -v -race -count=1 -coverprofile=$(COVERPROFILE) -failfast -p 1 -covermode=atomic  ./...

#: clean up from builds and tests
clean:
	@echo "+++ clean up, clean up, everybody, everywhere"
	rm -rf $(ARTIFACTS_DIR)
	rm -rf $(COVERPROFILE)

check_defined = \
    $(strip $(foreach 1,$1, \
        $(call __check_defined,$1,$(strip $(value 2)))))
__check_defined = \
    $(if $(value $1),, \
        $(error Undefined $1$(if $2, ($2))$(if $(value @), \
                required by target `$@')))

