//go:build rie

package rie

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
// network access, so a failure to fetch skips rather than fails: an offline
// machine shouldn't look like a broken extension.
func buildEmulator(t *testing.T) string {
	t.Helper()
	repo, ref := emulatorSource()

	cache := filepath.Join(os.TempDir(), "hny-lambda-ext-emulator", ref)
	binary := filepath.Join(cache, "aws-lambda-rie")
	if _, err := os.Stat(binary); err == nil {
		return binary
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatalf("preparing the emulator cache: %v", err)
	}

	checkout := filepath.Join(cache, "src")
	if _, err := os.Stat(filepath.Join(checkout, "go.mod")); err != nil {
		if out, err := exec.Command("git", "clone", "--quiet", repo, checkout).CombinedOutput(); err != nil {
			t.Skipf("could not fetch the emulator from %s (offline?): %v: %s", repo, err, out)
		}
	}
	if out, err := exec.Command("git", "-C", checkout, "checkout", "--quiet", ref).CombinedOutput(); err != nil {
		t.Skipf("could not check out emulator ref %s: %v: %s", ref, err, out)
	}

	build := exec.Command("go", "build", "-o", binary, "./cmd/aws-lambda-rie")
	build.Dir = checkout
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the emulator: %v: %s", err, out)
	}

	fmt.Fprintf(os.Stderr, "built emulator %s from %s\n", ref[:12], repo)
	return binary
}
