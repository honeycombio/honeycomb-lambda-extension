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


def main() -> int:
    # --count reports how many function-type messages a dump holds, so the
    # capture script can keep invoking until the function's stdout shows up.
    if sys.argv[1] == "--count":
        combined = decode(sys.argv[2])
        print(sum(1 for msg in combined if msg.get("type") == "function"))
        return 0

    raw_path, out_path, log_format = sys.argv[1], sys.argv[2], sys.argv[3]
    combined = decode(raw_path)
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
