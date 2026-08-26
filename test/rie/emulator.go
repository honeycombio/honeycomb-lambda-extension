//go:build rie

package rie

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// The Lambda base image bundles an emulator whose Telemetry API is a stub: it
// answers a subscription with a 2xx and then delivers nothing, so translation
// cannot be exercised against it. These tests build an emulator that implements
// the API instead, pinned to a commit so a run is reproducible.
//
// Override emulatorRepo to test against a different build, including a local
// checkout: EMULATOR_REPO=/path/to/checkout EMULATOR_REF=HEAD make test-rie
const (
	defaultEmulatorRepo = "https://github.com/lizthegrey/aws-lambda-runtime-interface-emulator.git"
	defaultEmulatorRef  = "0503f0ae6bc0c760ed6d62939a2b08cadbcce999"
)

func emulatorSource() (repo, ref string) {
	repo, ref = defaultEmulatorRepo, defaultEmulatorRef
	if override := os.Getenv("EMULATOR_REPO"); override != "" {
		repo = override
	}
	if override := os.Getenv("EMULATOR_REF"); override != "" {
		ref = override
	}
	return repo, ref
}

// buildEmulator returns the path to an emulator binary built for the container,
// fetching and compiling it once per ref and caching the result. Cloning needs
// network access, so a failure to fetch is reported as a skip reason rather
// than an error: an offline machine shouldn't look like a broken extension.
func buildEmulator() (binary, skipReason string, err error) {
	repo, ref := emulatorSource()

	// Keyed by architecture as well as ref, since the binary is built for the
	// container and a cache shared across architectures would be wrong.
	cache := filepath.Join(os.TempDir(), "hny-lambda-ext-emulator", ref+"-"+runtime.GOARCH)
	binary = filepath.Join(cache, "aws-lambda-rie")
	if _, err := os.Stat(binary); err == nil {
		return binary, "", nil
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", "", fmt.Errorf("preparing the emulator cache: %w", err)
	}

	checkout := filepath.Join(cache, "src")
	if _, err := os.Stat(filepath.Join(checkout, "go.mod")); err != nil {
		if out, err := exec.Command("git", "clone", "--quiet", repo, checkout).CombinedOutput(); err != nil {
			return "", fmt.Sprintf("could not fetch the emulator from %s (offline?): %v: %s", repo, err, out), nil
		}
	}
	if out, err := exec.Command("git", "-C", checkout, "checkout", "--quiet", ref).CombinedOutput(); err != nil {
		return "", fmt.Sprintf("could not check out emulator ref %s: %v: %s", ref, err, out), nil
	}

	build := exec.Command("go", "build", "-o", binary, "./cmd/aws-lambda-rie")
	build.Dir = checkout
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("building the emulator: %v: %s", err, out)
	}

	fmt.Fprintf(os.Stderr, "built emulator %s from %s\n", ref[:12], repo)
	return binary, "", nil
}
