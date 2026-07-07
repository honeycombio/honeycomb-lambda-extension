#!/bin/bash

set -x

artifact_dir=~/artifacts/linux
for arch in x86_64 arm64; do
    if [ ! -f "${artifact_dir}/extension-${arch}.zip" ]; then
        echo "${arch} extension does not exist, cannot publish."
        exit 1;
    fi
done

VERSION="${CIRCLE_TAG:-$(make version)}"
# lambda layer names must match a regex: [a-zA-Z0-9-_]+)|[a-zA-Z0-9-_]+
# turn periods into dashes and cry into your coffee
VERSION=$(echo ${VERSION} | tr '.' '-')

EXTENSION_NAME="honeycomb-lambda-extension"

# Region list verified against the publishing account 2026/07/07: every region
# the account has enabled (ec2 describe-regions) now supports arm64 Lambda
# (probed per region with list-layer-versions --compatible-architecture arm64).
# Regions absent here (il-central-1, ca-west-1, mx-central-1, ap-east-2,
# ap-southeast-5/6/7) are not opted in on the publishing account.
#
# regions with x86_64 only support
REGIONS_NO_ARM=(
)
# Regions with x86_64 & arm64
REGIONS_WITH_ARM=(
    af-south-1
    ap-east-1
    ap-northeast-1
    ap-northeast-2
    ap-northeast-3
    ap-south-1
    ap-south-2
    ap-southeast-1
    ap-southeast-2
    ap-southeast-3
    ap-southeast-4
    ca-central-1
    eu-central-1
    eu-central-2
    eu-north-1
    eu-south-1
    eu-south-2
    eu-west-1
    eu-west-2
    eu-west-3
    me-central-1
    me-south-1
    sa-east-1
    us-east-1
    us-east-2
    us-west-1
    us-west-2
)


results_dir="publishing"
mkdir -p ${results_dir}

### x86_64 ###

layer_name_x86_64="${EXTENSION_NAME}-x86_64-${VERSION}"

for region in ${REGIONS_WITH_ARM[@]}; do
    id="x86_64-${region}"
    publish_results_json="${results_dir}/publish-${id}.json"
    permit_results_json="${results_dir}/permit-${id}.json"
    aws lambda publish-layer-version \
        --layer-name $layer_name_x86_64 \
        --compatible-architectures x86_64 \
        --region $region \
        --zip-file "fileb://${artifact_dir}/extension-x86_64.zip" \
        --no-cli-pager \
        > "${publish_results_json}"
    layer_version=`jq -r '.Version' ${publish_results_json}`
    aws --region $region lambda add-layer-version-permission --layer-name $layer_name_x86_64 \
        --version-number $layer_version --statement-id "$EXTENSION_NAME-x86_64-$layer_version-$region" \
        --principal "*" --action lambda:GetLayerVersion --no-cli-pager \
        > "${permit_results_json}"
done

for region in ${REGIONS_NO_ARM[@]}; do
    id="x86_64-${region}"
    publish_results_json="${results_dir}/publish-${id}.json"
    permit_results_json="${results_dir}/permit-${id}.json"
    aws lambda publish-layer-version \
        --layer-name $layer_name_x86_64 \
        --region $region \
        --zip-file "fileb://${artifact_dir}/extension-x86_64.zip" \
        --no-cli-pager \
        > "${publish_results_json}"
    layer_version=`jq -r '.Version' ${publish_results_json}`
    aws --region $region lambda add-layer-version-permission --layer-name $layer_name_x86_64 \
        --version-number $layer_version --statement-id "$EXTENSION_NAME-x86_64-$layer_version-$region" \
        --principal "*" --action lambda:GetLayerVersion --no-cli-pager \
        > "${permit_results_json}"
done

### arm64 ###

layer_name_arm64="${EXTENSION_NAME}-arm64-${VERSION}"

for region in ${REGIONS_WITH_ARM[@]}; do
    id="arm64-${region}"
    publish_results_json="${results_dir}/publish-${id}.json"
    permit_results_json="${results_dir}/permit-${id}.json"
    aws lambda publish-layer-version \
        --layer-name $layer_name_arm64 \
        --compatible-architectures arm64 \
        --region $region \
        --zip-file "fileb://${artifact_dir}/extension-arm64.zip" \
        --no-cli-pager \
        > "${publish_results_json}"
    layer_version=`jq -r '.Version' ${publish_results_json}`
    aws --region $region lambda add-layer-version-permission --layer-name $layer_name_arm64 \
        --version-number $layer_version --statement-id "$EXTENSION_NAME-arm64-$layer_version-$region" \
        --principal "*" --action lambda:GetLayerVersion --no-cli-pager \
        > "${permit_results_json}"
done

echo ""
echo "Published Layer Versions:"
echo ""
jq '{ region: (.LayerArn | split(":")[3]),
        arch: (.LayerArn | split(":")[6] | split("-")[3]),
        arn: .LayerVersionArn
    }' publishing/publish-*.json
