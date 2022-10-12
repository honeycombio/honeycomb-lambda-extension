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

.PHONY: test
#: run the tests!
test:
ifeq (, $(shell which gotestsum))
	@echo " ***"
	@echo "Running with standard go test because gotestsum was not found on PATH. Consider installing gotestsum for friendlier test output!"
	@echo " ***"
	go test -race ./...
else
	gotestsum --junitfile unit-tests.xml --format testname -- -race ./...
endif

# target directory for artifact builds
ARTIFACT_DIR := artifacts
$(ARTIFACT_DIR):
	mkdir -p $@

BUILD_DIR := $(ARTIFACT_DIR)/$(GOOS)
$(BUILD_DIR):
	mkdir -p $@

# List of the Go source files; the build target will then know if these are newer than an executable present.
GO_SOURCES := go.mod go.sum $(wildcard *.go) $(wildcard */*.go)
ldflags := "-X main.version=$(CIRCLE_TAG)"

$(BUILD_DIR)/honeycomb-lambda-extension-arm64: $(GO_SOURCES) | $(BUILD_DIR)
	@echo "\n*** Building honeycomb-lambda-extension for ${GOOS}/arm64"
	GOOS=${GOOS} GOARCH=arm64 go build -ldflags ${ldflags} -o $@ .

$(BUILD_DIR)/honeycomb-lambda-extension-x86_64: $(GO_SOURCES) | $(BUILD_DIR)
	@echo "\n*** Building honeycomb-lambda-extension for ${GOOS}/x86_64"
	GOOS=${GOOS} GOARCH=amd64 go build -ldflags ${ldflags} -o $@ .

.PHONY: build
#: build the executables
build: $(BUILD_DIR)/honeycomb-lambda-extension-arm64 $(BUILD_DIR)/honeycomb-lambda-extension-x86_64

### ZIPs for layer publishing
#
# Linux is the only supported OS.
#
# The ZIP file for the content of a lambda layers a.k.a. extention MUST have:
#   * an extensions/ directory
#   * the executable that is the extension located within the extensions/ directory
#
# some of the Make automatic variables in use in these recipes:
#   $(@D) - the directory portion of the target, e.g. artifacts/linux/thingie.zip $(@D) == artifacts/linux
#   $(@F) - the file portion of the target, e.g. artifacts/linux/thingie.zip, $(@F) == thingie.zip
#   $<    - the first prerequisite, in this case the executable being put into the zip file
$(ARTIFACT_DIR)/linux/extension-arm64.zip: $(ARTIFACT_DIR)/linux/honeycomb-lambda-extension-arm64
	@echo "\n*** Packaging honeycomb-lambda-extension for linux into layer contents zipfile"
	rm -rf $(@D)/extensions
	mkdir -p $(@D)/extensions
	cp $< $(@D)/extensions
	cd $(@D) && zip --move --recurse-paths $(@F) extensions

$(ARTIFACT_DIR)/linux/extension-x86_64.zip: $(ARTIFACT_DIR)/linux/honeycomb-lambda-extension-x86_64
	@echo "\n*** Packaging honeycomb-lambda-extension for linux into layer contents zipfile"
	rm -rf $(@D)/extensions
	mkdir -p $(@D)/extensions
	cp $< $(@D)/extensions
	cd $(@D) && zip --move --recurse-paths $(@F) extensions

#: build the zipfiles destined to be published as layer contents (GOOS=linux only)
ifeq ($(GOOS),linux)
zips: $(ARTIFACT_DIR)/linux/extension-arm64.zip $(ARTIFACT_DIR)/linux/extension-x86_64.zip
else
zips:
	@echo "\n*** GOOS is set to ${GOOS}. Zips destined for publishing as a layer can only be for linux."
	exit 1
endif

.PHONY: publish-sandbox
#: for serious, don't use this as-is for real publishing
publish_sandbox: are_you_sure arch_required region_required zips
	@echo "\n*** Publishing honeycomb-lambda-extension-${AWS_ARCH} to ${AWS_REGION}"
	aws lambda publish-layer-version \
		--layer-name honeycomb-lambda-extension-${AWS_ARCH} \
		--region ${AWS_REGION} \
		--compatible-architectures ${AWS_ARCH} \
		--zip-file "fileb://${ARTIFACT_DIR}/linux/extension-${AWS_ARCH}.zip" \
		--no-cli-pager

#: clean up the workspace
clean:
	rm -rf artifacts

### Utilities

.PHONY: region_required
region_required:
	@:$(call check_defined, AWS_REGION, the region to publish to)

.PHONY: arch_required
arch_required:
	@:$(call check_defined, AWS_ARCH, the architecture to publish)

.PHONY: are_you_sure
are_you_sure:
		@( read -p "Are you sure?!? [y/N]: " sure && case "$$sure" in [yY]) true;; *) false;; esac )

check_defined = \
    $(strip $(foreach 1,$1, \
        $(call __check_defined,$1,$(strip $(value 2)))))
__check_defined = \
    $(if $(value $1),, \
        $(error Undefined $1$(if $2, ($2))$(if $(value @), \
                required by target `$@')))
