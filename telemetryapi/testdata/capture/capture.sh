#!/usr/bin/env bash
# Captures what the Lambda Telemetry API really delivers, so the handler's tests
# can replay genuine payloads instead of hand-written approximations.
#
# Deploys a throwaway function and layer, invokes it once under each of Lambda's
# two log formats, pulls the captured request bodies back out of CloudWatch, and
# deletes everything it created. Writes one golden file per log format into the
# testdata directory above this one.
#
# Requires: aws cli, go, zip, python3, jq. Everything it creates is named with
# the prefix below, and teardown runs on exit even if a step fails.
set -euo pipefail

PROFILE="${AWS_PROFILE:-platform-us}"
REGION="${AWS_REGION:-us-east-2}"
NAME="${NAME:-hny-lambda-ext-capture}"
ROLE_ARN="${ROLE_ARN:-arn:aws:iam::702835727665:role/lambda-basic}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TESTDATA="$(cd "$HERE/.." && pwd)"
WORK="$(mktemp -d)"
aws() { command aws --profile "$PROFILE" --region "$REGION" "$@"; }

teardown() {
	if [ "${KEEP:-0}" = "1" ]; then
		echo "--- KEEP=1, leaving $NAME in place for diagnosis"
		return
	fi
	echo "--- tearing down"
	aws lambda delete-function --function-name "$NAME" 2>/dev/null || true
	for version in $(aws lambda list-layer-versions --layer-name "$NAME" \
		--query 'LayerVersions[].Version' --output text 2>/dev/null || true); do
		aws lambda delete-layer-version --layer-name "$NAME" --version-number "$version" 2>/dev/null || true
	done
	aws logs delete-log-group --log-group-name "/aws/lambda/$NAME" 2>/dev/null || true
	rm -rf "$WORK"
	echo "--- teardown complete; confirming nothing is left"
	if aws lambda get-function --function-name "$NAME" >/dev/null 2>&1; then
		echo "WARNING: function $NAME still exists"
	else
		echo "    function: gone"
	fi
	# A layer with no versions holds nothing; list succeeds either way, so count.
	remaining=$(aws lambda list-layer-versions --layer-name "$NAME" \
		--query 'length(LayerVersions)' --output text 2>/dev/null || echo 0)
	if [ "$remaining" = "0" ]; then
		echo "    layer versions: none remaining"
	else
		echo "WARNING: $remaining layer version(s) of $NAME remain"
	fi
}
trap teardown EXIT

echo "--- building for linux/amd64"
(cd "$HERE/extension" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$WORK/extensions/$NAME" .)
(cd "$HERE/handler" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$WORK/bootstrap" .)

echo "--- packaging"
(cd "$WORK" && zip -qr layer.zip extensions && zip -q function.zip bootstrap)

echo "--- publishing layer"
LAYER_ARN=$(aws lambda publish-layer-version --layer-name "$NAME" \
	--zip-file "fileb://$WORK/layer.zip" \
	--compatible-runtimes provided.al2023 \
	--query 'LayerVersionArn' --output text)
echo "    $LAYER_ARN"

echo "--- removing any leftovers from an interrupted run"
aws lambda delete-function --function-name "$NAME" 2>/dev/null && \
	aws lambda wait function-not-exists --function-name "$NAME" 2>/dev/null || true

echo "--- creating function"
aws lambda create-function --function-name "$NAME" \
	--runtime provided.al2023 --handler bootstrap --role "$ROLE_ARN" \
	--zip-file "fileb://$WORK/function.zip" --layers "$LAYER_ARN" \
	--timeout 30 --architectures x86_64 >/dev/null
aws lambda wait function-active-v2 --function-name "$NAME"

for format in Text JSON; do
	echo "--- setting log format to $format"
	aws lambda update-function-configuration --function-name "$NAME" \
		--logging-config "LogFormat=$format" >/dev/null
	aws lambda wait function-updated-v2 --function-name "$NAME"

	# An invocation stays thawed until every extension asks for its next event, so
	# the capture extension can and does receive the invoke's telemetry within it.
	# A second invoke is still needed for platform.report, which is only emitted
	# once the invoke has fully completed. Only the first invoke emits payloads, so
	# extra invokes can't duplicate anything.
	START_MS=$(python3 -c 'import time;print(int(time.time()*1000)-2000)')
	for pass in 1 2; do
		echo "--- invoking (pass $pass)"
		# raw-in-base64-out because AWS CLI v2 otherwise expects the payload
		# itself to be base64.
		aws lambda invoke --function-name "$NAME" --payload '{}' \
			--cli-binary-format raw-in-base64-out \
			--query 'FunctionError' --output text \
			"$WORK/out-$format-$pass.json" >"$WORK/err-$format-$pass.txt"
		if [ "$(cat "$WORK/err-$format-$pass.txt")" != "None" ]; then
			echo "    invoke reported an error: $(cat "$WORK/out-$format-$pass.json")"
			exit 1
		fi
		sleep 3
	done

	echo "--- collecting captures"
	# Telemetry lands only after the environment thaws, so keep invoking until the
	# function's own stdout actually appears rather than settling for init-phase
	# messages that arrive first.
	for attempt in 1 2 3 4 5 6; do
		aws logs filter-log-events --log-group-name "/aws/lambda/$NAME" \
			--start-time "$START_MS" \
			--filter-pattern '"CAPTURE:"' --query 'events[].message' --output json \
			>"$WORK/raw-$format.json" 2>/dev/null || echo '[]' >"$WORK/raw-$format.json"
		if [ "$(python3 "$HERE/collect.py" --ready "$WORK/raw-$format.json")" = "1" ]; then
			break
		fi
		echo "    capture incomplete (attempt $attempt); invoking again"
		aws lambda invoke --function-name "$NAME" --payload '{}' \
			--cli-binary-format raw-in-base64-out "$WORK/nudge.json" >/dev/null
		sleep 10
	done

	lower=$(printf '%s' "$format" | tr '[:upper:]' '[:lower:]')
	python3 "$HERE/collect.py" "$WORK/raw-$format.json" \
		"$TESTDATA/telemetry-api-$lower-log-format.json" "$format"
done

echo "--- done; goldens written to $TESTDATA"
