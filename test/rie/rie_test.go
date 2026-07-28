//go:build rie

// Package rie runs the extension inside the AWS Lambda Runtime Interface
// Emulator, in the real Lambda base image, driven by the real Extensions API.
//
// What this covers is the lifecycle: that the platform launches the extension
// from /opt/extensions, that registration succeeds, that INVOKE and SHUTDOWN
// arrive and are handled, and that the process neither panics nor blocks the
// function. That is the class of failure that made the extension unusable on
// Lambda Managed Instances, and none of it is reachable from a unit test.
//
// What this cannot cover is telemetry translation. The emulator answers a
// Telemetry API subscription with a 2xx whose body reads Telemetry.NotSupported
// and then delivers nothing, so no span can be produced here however the
// extension behaves. That is what testdata's captured payloads and the replay
// tests are for; a green run of this suite says nothing about OTLP.
//
// Two emulator behaviors shape these tests. It initializes the runtime and its
// extensions lazily on the first invocation rather than at container start, so
// there is nothing to wait for before invoking. And because its subscription
// response is a 2xx, no test here can exercise what the extension does with a
// subscription that actually fails.
//
// Requires Docker. Excluded from the default build by the rie tag; run with
// `make test-rie`.
package rie

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	image         = "honeycomb-lambda-extension-rie:test"
	containerName = "hny-lambda-ext-rie-test"
	hostPort      = "9101"
)

func TestMain(m *testing.M) {
	if err := buildImage(); err != nil {
		fmt.Fprintf(os.Stderr, "building test image: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// buildImage compiles the extension and a stub handler for linux/amd64 and bakes
// them into the Lambda base image at the paths the platform expects.
func buildImage() error {
	work, err := os.MkdirTemp("", "hny-rie")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	for _, target := range []struct{ pkg, out string }{
		{"../..", "extension"},
		{"./handler", "bootstrap"},
	} {
		build := exec.Command("go", "build", "-o", filepath.Join(work, target.out), target.pkg)
		build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			return fmt.Errorf("building %s: %v: %s", target.pkg, err, out)
		}
	}

	dockerfile := `FROM public.ecr.aws/lambda/provided:al2023
COPY extension /opt/extensions/honeycomb-lambda-extension
COPY bootstrap /var/runtime/bootstrap
CMD ["bootstrap"]
`
	if err := os.WriteFile(filepath.Join(work, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}

	build := exec.Command("docker", "build", "-q", "-t", image, work)
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build: %v: %s", err, out)
	}
	return nil
}

// run starts the container with extra environment, invokes the function once, and
// returns the invocation's response body alongside everything logged.
func run(t *testing.T, env map[string]string) (response string, logs string) {
	t.Helper()

	args := []string{"run", "-d", "--name", containerName, "-p", hostPort + ":8080",
		"-e", "LIBHONEY_API_KEY=abc123def456ghi789jkl012m",
		"-e", "LIBHONEY_DATASET=rie-test-dataset",
		// Nothing should be delivered, but point the publisher somewhere closed
		// rather than at Honeycomb in case that assumption is ever wrong.
		"-e", "LIBHONEY_API_HOST=http://127.0.0.1:1",
		"-e", "HONEYCOMB_DEBUG=true",
	}
	for key, value := range env {
		args = append(args, "-e", key+"="+value)
	}
	args = append(args, image)

	exec.Command("docker", "rm", "-f", containerName).Run()
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v: %s", err, out)
	}
	t.Cleanup(func() {
		exec.Command("docker", "rm", "-f", containerName).Run()
	})

	// The emulator initializes the runtime and its extensions lazily, on the first
	// invocation rather than at container start, so there is nothing to wait for
	// beforehand: the invoke is what causes the extension to be launched at all.
	body := invokeWhenReady(t)

	// Let the extension finish its post-invoke window before reading logs.
	time.Sleep(2 * time.Second)
	return body, containerLogs(t)
}

// invokeWhenReady posts an invocation, retrying until the emulator is listening.
func invokeWhenReady(t *testing.T) string {
	t.Helper()
	url := fmt.Sprintf("http://localhost:%s/2015-03-31/functions/function/invocations", hostPort)

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		body, err := exec.Command("curl", "-sS", "-XPOST", url, "-d", "{}").Output()
		if err == nil && len(body) > 0 {
			return string(body)
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("emulator never served an invocation within 60s; logs:\n%s", containerLogs(t))
	return ""
}

func containerLogs(t *testing.T) string {
	t.Helper()
	out, _ := exec.Command("docker", "logs", containerName).CombinedOutput()
	return string(out)
}

// The platform has to find the extension, start it, and let it register; then the
// invocation has to complete normally with the extension in the loop.
func TestExtensionRegistersAndInvocationSucceeds(t *testing.T) {
	response, logs := run(t, nil)

	if !strings.Contains(response, "ok") {
		t.Errorf("function did not return successfully: %q\nlogs:\n%s", response, logs)
	}
	for _, want := range []string{
		"External agent honeycomb-lambda-extension",
		"registered, subscribed to [INVOKE SHUTDOWN]",
		"Received INVOKE event.",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("expected logs to contain %q\nlogs:\n%s", want, logs)
		}
	}
	assertHealthy(t, logs)
}

// The emulator acknowledges a Telemetry API subscription with a 2xx whose body
// says Telemetry.NotSupported, and then delivers nothing — the "non-committal
// response" its own source describes. The extension has to keep working when
// subscribed telemetry simply never arrives: keep processing INVOKE, and leave
// the function unaffected.
//
// Note what this does not test. Subscribe only treats status >= 400 as an error,
// so this path never produces a subscription error and cannot show what the
// extension does with one. It also means the extension cannot presently tell an
// accepted subscription from a silently ineffective one.
func TestTelemetryNeverArrivingIsHarmless(t *testing.T) {
	response, logs := run(t, nil)

	if !strings.Contains(logs, "Telemetry.NotSupported") {
		t.Skipf("emulator no longer reports Telemetry.NotSupported; it may now implement the API\nlogs:\n%s", logs)
	}
	if !strings.Contains(response, "ok") {
		t.Errorf("undelivered telemetry must not break the function: %q", response)
	}
	if !strings.Contains(logs, "Received INVOKE event.") {
		t.Errorf("extension stopped processing events\nlogs:\n%s", logs)
	}
	assertHealthy(t, logs)
}

// On Lambda Managed Instances only SHUTDOWN may be registered for, because one
// execution environment serves concurrent invocations. Registering for INVOKE
// there is rejected, and treating that as fatal is what broke the extension.
func TestManagedInstancesRegistration(t *testing.T) {
	response, logs := run(t, map[string]string{
		"AWS_LAMBDA_INITIALIZATION_TYPE": "lambda-managed-instances",
	})

	if !strings.Contains(logs, "registered, subscribed to [SHUTDOWN]") {
		t.Errorf("expected a SHUTDOWN-only registration on managed instances\nlogs:\n%s", logs)
	}
	if !strings.Contains(response, "ok") {
		t.Errorf("function did not return successfully: %q", response)
	}
	assertHealthy(t, logs)
}

// assertHealthy fails on the ways the extension can take the sandbox down with
// it, rather than only checking for the things that should be present.
func assertHealthy(t *testing.T, logs string) {
	t.Helper()
	for _, unwanted := range []string{
		"panic:",
		"ExtensionInitError",
		"Sandbox.Failure",
		"failed to launch",
		"Init failed",
	} {
		if strings.Contains(logs, unwanted) {
			t.Errorf("logs report %q\nlogs:\n%s", unwanted, logs)
		}
	}
}
