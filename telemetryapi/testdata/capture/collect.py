#!/usr/bin/env python3
"""Turns CloudWatch log lines from the capture extension into a golden file.

Each captured line is CAPTURE:<base64 of one Telemetry API request body>. Every
body is a JSON array of telemetry messages; they are concatenated into a single
array so the replay test can post the whole capture as one batch.

Usage: collect.py <raw-cloudwatch-events.json> <output.json> <log-format>
"""
import base64
import json
import sys


def decode(raw_path):
    """Yields every telemetry message across all captures in a CloudWatch dump."""
    with open(raw_path) as handle:
        messages = json.load(handle)

    bodies = []
    for message in messages:
        for line in message.splitlines():
            marker = line.find("CAPTURE:")
            if marker == -1:
                continue
            encoded = line[marker + len("CAPTURE:"):].strip()
            try:
                bodies.append(json.loads(base64.b64decode(encoded)))
            except (ValueError, json.JSONDecodeError) as err:
                print(f"skipping undecodable capture: {err}", file=sys.stderr)

    return [msg for body in bodies for msg in body]


def dedupe(messages):
    """Keeps one message per distinct type-and-record.

    A capture can span several execution environments, since changing the log
    format replaces them, and each fresh environment re-emits every payload. The
    golden only needs one real example of each shape, so identical content is
    collapsed and the earliest timestamp for it is kept.
    """
    seen = {}
    for msg in messages:
        key = (msg.get("type"), json.dumps(msg.get("record"), sort_keys=True))
        if key not in seen or msg.get("time", "") < seen[key].get("time", ""):
            seen[key] = msg
    return list(seen.values())


def ready(messages):
    """Whether a capture holds everything the goldens need.

    platform.report is only emitted once an invocation has fully completed, so it
    cannot arrive during the invoke that produced it, however long an extension
    holds the window open.
    """
    have_function = any(msg.get("type") == "function" for msg in messages)
    have_report = any(msg.get("type") == "platform.report" for msg in messages)
    return have_function and have_report


def main() -> int:
    # --ready tells the capture script whether to keep invoking.
    if sys.argv[1] == "--ready":
        print("1" if ready(decode(sys.argv[2])) else "0")
        return 0

    raw_path, out_path, log_format = sys.argv[1], sys.argv[2], sys.argv[3]
    combined = dedupe(decode(raw_path))
    if not combined:
        print("no telemetry captured; refusing to write an empty golden", file=sys.stderr)
        return 1

    # Sorting by time keeps the file stable across runs, so a re-capture produces
    # a reviewable diff rather than a reshuffle.
    combined.sort(key=lambda msg: (msg.get("time", ""), msg.get("type", "")))

    with open(out_path, "w") as handle:
        json.dump(combined, handle, indent=2, sort_keys=True)
        handle.write("\n")

    types = {}
    for msg in combined:
        types[msg.get("type", "?")] = types.get(msg.get("type", "?"), 0) + 1
    print(f"{log_format}: wrote {len(combined)} messages to {out_path}")
    for name in sorted(types):
        print(f"    {name}: {types[name]}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
