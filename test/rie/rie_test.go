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
// It also covers translation end to end: the function writes each payload shape
// to real stdout, the platform delivers it, and the events the extension sends
// are decoded and asserted on. The emulator bundled in the Lambda base image
// cannot do this -- its Telemetry API is a stub that accepts a subscription and
// delivers nothing -- so these tests build one that implements the API, pinned in
// emulator.go. Anything the emulator gets wrong is still emulation: the captured
// payloads in testdata remain the authority on what Lambda really sends.
//
// The emulator initializes the runtime and its extensions lazily on the first
// invocation rather than at container start, so there is nothing to wait for
// before invoking.
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
	"sync"
	"testing"
	"time"
)

// invokeResponse is what the test handler returns from a successful invocation.
const invokeResponse = "captured"

const (
	image         = "honeycomb-lambda-extension-rie:test"
	containerName = "hny-lambda-ext-rie-test"
	hostPort      = "9101"
)

// The image is built once, by whichever test needs it first, because building
// the emulator needs a *testing.T to skip on when the network is unavailable.
var buildOnce sync.Once

func ensureImage(t *testing.T) {
	t.Helper()
	buildOnce.Do(func() {
		if err := buildImage(t); err != nil {
			t.Fatalf("building the test image: %v", err)
		}
	})
}

func copyFile(from, to string) error {
	content, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, content, 0o755)
}

// buildImage compiles the extension and a stub handler for linux/amd64 and bakes
// them into the Lambda base image at the paths the platform expects.
func buildImage(t *testing.T) error {
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

	emulator := buildEmulator(t)
	if err := copyFile(emulator, filepath.Join(work, "aws-lambda-rie")); err != nil {
		return err
	}

	dockerfile := `FROM public.ecr.aws/lambda/provided:al2023
COPY aws-lambda-rie /usr/local/bin/aws-lambda-rie
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
	_, response, logs = runWithSink(t, env)
	return response, logs
}

// runWithSink starts the container with a stand-in Honeycomb API, invokes the
// function once, and returns everything the extension delivered.
func runWithSink(t *testing.T, env map[string]string) (*sink, string, string) {
	t.Helper()
	ensureImage(t)

	deliveries := startSink(t)

	args := []string{"run", "-d", "--name", containerName, "-p", hostPort + ":8080",
		// host-gateway keeps the sink reachable where host.docker.internal is not
		// resolved for us, such as Docker on Linux.
		"--add-host", "host.docker.internal:host-gateway",
		"-e", "LIBHONEY_API_KEY=abc123def456ghi789jkl012m",
		"-e", "LIBHONEY_DATASET=rie-test-dataset",
		"-e", "LIBHONEY_API_HOST=http://host.docker.internal:" + deliveries.port(),
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

	// Telemetry is delivered in the extension's post-invoke window, and batches
	// are sent asynchronously after that.
	time.Sleep(6 * time.Second)
	return deliveries, body, containerLogs(t)
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

	if !strings.Contains(response, invokeResponse) {
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
	if !strings.Contains(response, invokeResponse) {
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

// The whole point of the extension: a function writes OTLP to stdout and spans
// arrive at Honeycomb. Every recognized payload shape is exercised here through
// the real platform, which is the one thing unit tests cannot do.
func TestTranslatesEveryPayloadShape(t *testing.T) {
	deliveries, response, logs := runWithSink(t, nil)

	if !strings.Contains(response, invokeResponse) {
		t.Fatalf("function did not return successfully: %q\nlogs:\n%s", response, logs)
	}
	if len(deliveries.delivered()) == 0 {
		t.Fatalf("the extension delivered nothing; the emulator may not be serving telemetry\nlogs:\n%s", logs)
	}

	byName := deliveries.byName()

	t.Run("OTLP/JSON traces", func(t *testing.T) {
		span, ok := byName["handler"]
		if !ok {
			t.Fatalf("no translated span named handler; delivered %d events", len(deliveries.delivered()))
		}
		if span.Dataset != "rie-func" {
			t.Errorf("dataset = %q, want rie-func from service.name", span.Dataset)
		}
		if got := span.Data["trace.trace_id"]; got != "5b8efff798038103d269b633813fc60c" {
			t.Errorf("trace.trace_id = %v", got)
		}
		if got := span.Data["span.kind"]; got != "server" {
			t.Errorf("span.kind = %v, want server", got)
		}
		if got := span.Data["library.name"]; got != "rie-instrumentation" {
			t.Errorf("library.name = %v", got)
		}
	})

	t.Run("OTLP/JSON logs", func(t *testing.T) {
		var found bool
		for _, event := range deliveries.delivered() {
			if event.Data["body"] == "captured log record" {
				found = true
				if event.Dataset != "rie-func" {
					t.Errorf("dataset = %q, want rie-func", event.Dataset)
				}
				if got := event.Data["severity_text"]; got != "ERROR" {
					t.Errorf("severity_text = %v", got)
				}
			}
		}
		if !found {
			t.Error("the OTLP log record was not translated")
		}
	})

	t.Run("otlp-stdout envelope", func(t *testing.T) {
		// The envelope carries a second copy of the same span, so the direct
		// payload and the compressed one together produce two.
		if got := deliveries.spansNamed("handler"); got != 2 {
			t.Errorf("spans named handler = %d, want 2 (one direct, one from the envelope)", got)
		}
	})

	t.Run("libhoney envelope still works", func(t *testing.T) {
		span, ok := byName["beeline-span"]
		if !ok {
			t.Fatal("the libhoney envelope was not unwrapped")
		}
		if span.Dataset != "rie-test-dataset" {
			t.Errorf("dataset = %q, want the configured dataset", span.Dataset)
		}
		if _, nested := span.Data["data"]; nested {
			t.Error("the envelope should be unwrapped, not nested")
		}
	})

	t.Run("plain stdout is still a log line", func(t *testing.T) {
		var found bool
		for _, event := range deliveries.delivered() {
			if record, ok := event.Data["record"].(string); ok && strings.Contains(record, "an ordinary log line") {
				found = true
				if event.Dataset != "rie-test-dataset" {
					t.Errorf("dataset = %q, want the configured dataset", event.Dataset)
				}
			}
		}
		if !found {
			t.Error("plain stdout did not arrive as a record field")
		}
	})

	t.Run("platform telemetry is forwarded", func(t *testing.T) {
		seen := map[string]bool{}
		for _, event := range deliveries.delivered() {
			if kind, ok := event.Data["lambda_extension.type"].(string); ok {
				seen[kind] = true
			}
		}
		for _, kind := range []string{"platform.start", "platform.runtimeDone", "platform.initStart"} {
			if !seen[kind] {
				t.Errorf("expected %s to be forwarded; saw %v", kind, seen)
			}
		}
	})
}

// Translated telemetry must route by service.name while everything else stays in
// the configured dataset. Getting this wrong would scatter a customer's data.
func TestOnlyTranslatedTelemetryChangesDataset(t *testing.T) {
	deliveries, _, _ := runWithSink(t, nil)

	for _, event := range deliveries.delivered() {
		_, isSpan := event.Data["trace.trace_id"]
		_, isLogRecord := event.Data["severity_text"]
		translated := isSpan || isLogRecord

		switch {
		case translated && event.Dataset != "rie-func":
			t.Errorf("translated telemetry went to %q, want rie-func", event.Dataset)
		case !translated && event.Dataset != "rie-test-dataset":
			t.Errorf("untranslated event went to %q, want rie-test-dataset: %v", event.Dataset, event.Data)
		}
	}
}
