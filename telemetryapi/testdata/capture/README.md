# Telemetry API capture

The golden files in the directory above (`telemetry-api-*-log-format.json`) are
real Telemetry API request bodies, recorded from a Lambda function rather than
written by hand. `replay_test.go` posts them at the handler, which is what proves
the handler agrees with the platform instead of with our assumptions about it.

Regenerate them only when Lambda's delivery format changes or a new payload shape
needs covering. A capture takes a couple of minutes:

```
AWS_PROFILE=<profile> AWS_REGION=<region> ./capture.sh
```

The script deploys a throwaway function and layer named `hny-lambda-ext-capture`,
invokes it under each of Lambda's two log formats, pulls the captured bodies out
of CloudWatch, and deletes everything it created — including on failure, and it
prints confirmation that nothing is left behind. `KEEP=1` skips teardown when a
capture needs debugging. It reuses an existing execution role rather than
creating IAM; override with `ROLE_ARN`.

Three behaviors of the platform shape how this works:

- **An invocation isn't over when the runtime responds.** Lambda keeps the
  environment thawed until every registered extension asks for its next event,
  and only then freezes. That window is what an extension uses to flush, and it's
  when the invoke's telemetry is delivered. The capture extension therefore waits
  until telemetry stops arriving before asking for its next event; a tight
  next-event loop hands back permission to freeze before anything shows up, and
  captures only the init phase.
- **`platform.report` is emitted after the invocation fully completes**, so it
  can never arrive during the invoke that produced it, however long an extension
  holds the window open. The script keeps invoking until it has both function
  telemetry and a report.
- **The capture extension subscribes to `function` and `platform` but not
  `extension`.** Its captures are written to its own stdout, which is extension
  telemetry; subscribing to that would feed every capture back to itself.

Changing the log format replaces the execution environment, and each fresh
environment re-emits every payload, so captures are deduplicated by content —
one message per distinct type and record, keeping the earliest timestamp. That
makes a re-capture produce a reviewable diff rather than a reshuffle.

## What the current goldens do and don't cover

`handler/main.go` emits one line of each shape under test: OTLP/JSON traces,
OTLP/JSON logs, an `otlp-stdout` envelope, a libhoney/beeline envelope, and a
plain text line.

They were captured on the `provided.al2023` custom runtime, where a non-JSON
stdout line arrives as a bare string under **both** log formats. The
`{timestamp, level, message}` wrapper that the handler also unwraps did not
appear, so that path is still covered only by hand-written tests — it appears to
come from the managed runtimes' logging shims rather than from the platform, and
confirming that would mean capturing again on a managed runtime.

Worth noting what the capture did settle: under JSON log format a line that was
already JSON arrives as that object **verbatim**, with no platform keys merged
in. Extra keys would have made every OTLP payload fail to parse, since protojson
rejects unknown fields.
