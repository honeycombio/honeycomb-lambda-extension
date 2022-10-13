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

# Region list update from AWS Lambda pricing page as of 2022/10/12
#
# regions with x86_64 only support
# REGIONS_NO_ARCH=(me-central-1) # listed on pricing page, but we need enable this region before publishing to it.
REGIONS_NO_ARCH=()
# Regions with x86_64 & arm64
REGIONS_WITH_ARCH=(
    af-south-1
    ap-east-1
    ap-northeast-1
    ap-northeast-2
    ap-northeast-3
    ap-south-1
    ap-southeast-1
    ap-southeast-2
    ap-southeast-3
    ca-central-1
    eu-central-1
    eu-north-1
    eu-south-1
    eu-west-1
    eu-west-2
    eu-west-3
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

for region in ${REGIONS_WITH_ARCH[@]}; do
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

for region in ${REGIONS_NO_ARCH[@]}; do
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

for region in ${REGIONS_WITH_ARCH[@]}; do
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
