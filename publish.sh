#!/bin/bash

set -x

artifact_dir=~/artifacts/linux
for arch in x86_64 arm64; do
    if [ ! -f "${artifact_dir}/extension-${arch}.zip" ]; then
        echo "${arch} extension does not exist, cannot publish."
        exit 1;
    fi
done

if [[ "${CIRCLE_TAG}" == *dev ]]; then
    EXTENSION_NAME="honeycomb-lambda-extension-dev"
else
    EXTENSION_NAME="honeycomb-lambda-extension"
fi

REGIONS_NO_ARCH=(eu-north-1 us-west-1 eu-west-3 ap-northeast-2 sa-east-1 ca-central-1 af-south-1
                ap-east-1 eu-south-1 me-south-1 ap-southeast-3 ap-northeast-3)
REGIONS_WITH_ARCH=(ap-south-1 eu-west-2 us-east-1 eu-west-1 ap-northeast-1 ap-southeast-1
                   ap-southeast-2 eu-central-1 us-east-2 us-west-2)

### x86_64 ###

layer_name_x86_64="$EXTENSION_NAME-x86_64"

for region in ${REGIONS_WITH_ARCH[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name $layer_name_x86_64 \
        --compatible-architectures x86_64 \
        --region $region --zip-file "fileb://"${artifact_dir}/extension-x86_64.zip""`
    layer_version=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name $layer_name_x86_64 \
        --version-number $layer_version --statement-id "$EXTENSION_NAME-x86_64-$layer_version-$region" \
        --principal "*" --action lambda:GetLayerVersion
done

for region in ${REGIONS_NO_ARCH[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name $layer_name_x86_64 \
        --region $region --zip-file "fileb://${artifact_dir}/extension-x86_64.zip"`
    layer_version=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name $layer_name_x86_64 \
        --version-number $layer_version --statement-id "$EXTENSION_NAME-x86_64-$layer_version-$region" \
        --principal "*" --action lambda:GetLayerVersion
done

### arm64 ###

layer_name_arm64="$EXTENSION_NAME-arm64"

for region in ${REGIONS_WITH_ARCH[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name $layer_name_arm64 \
        --compatible-architectures arm64 \
        --region $region --zip-file "fileb://${artifact_dir}/extension-arm64.zip"`
    layer_version=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name $layer_name_arm64 \
        --version-number $layer_version --statement-id "$EXTENSION_NAME-arm64-$layer_version-$region" \
        --principal "*" --action lambda:GetLayerVersion
done
