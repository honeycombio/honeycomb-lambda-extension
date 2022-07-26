#!/bin/bash

set -x

if [[ "${CIRCLE_TAG}" == *dev ]]; then
    EXTENSION_NAME="honeycomb-lambda-extension-dev"
else
    EXTENSION_NAME="honeycomb-lambda-extension"
fi

REGIONS_NO_ARCH=(eu-north-1 us-west-1 eu-west-3 ap-northeast-2 sa-east-1 ca-central-1 af-south-1 ap-east-1 eu-south-1 me-south-1)
REGIONS_WITH_ARCH=(ap-south-1 eu-west-2 us-east-1 eu-west-1 ap-northeast-1 ap-southeast-1
                   ap-southeast-2 eu-central-1 us-east-2 us-west-2)

if [ ! -f ~/artifacts/honeycomb-lambda-extension-amd64 ]; then
    echo "amd64 extension does not exist, cannot publish."
    exit 1;
fi

if [ ! -f ~/artifacts/honeycomb-lambda-extension-arm64 ]; then
    echo "arm64 extension does not exist, cannot publish."
    exit 1;
fi

cd ~/artifacts

mkdir -p amd64/extensions
cp honeycomb-lambda-extension-amd64 amd64/extensions/
cd amd64
# the zipfile MUST contain a directory named "extensions"
# and that directory MUST contain the extension's executable
zip -r extension.zip extensions

for region in ${REGIONS_WITH_ARCH[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name "$EXTENSION_NAME-x86_64" \
        --compatible-architectures x86_64 \
        --region $region --zip-file "fileb://extension.zip"`
    VERSION=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name "$EXTENSION_NAME-x86_64" \
        --version-number $VERSION --statement-id "$EXTENSION_NAME-x86_64-$VERSION-$region" \
        --principal "*" --action lambda:GetLayerVersion
done

for region in ${REGIONS_NO_ARCH[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name "$EXTENSION_NAME-x86_64" \
        --region $region --zip-file "fileb://extension.zip"`
    VERSION=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name "$EXTENSION_NAME-x86_64" \
        --version-number $VERSION --statement-id "$EXTENSION_NAME-x86_64-$VERSION-$region" \
        --principal "*" --action lambda:GetLayerVersion
done

cd ~/artifacts

mkdir -p arm64/extensions
cp honeycomb-lambda-extension-arm64 arm64/extensions/
cd arm64
# the zipfile MUST contain a directory named "extensions"
# and that directory MUST contain the extension's executable
zip -r extension.zip extensions

for region in ${REGIONS_WITH_ARCH[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name "$EXTENSION_NAME-arm64" \
        --compatible-architectures arm64 \
        --region $region --zip-file "fileb://extension.zip"`
    VERSION=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name "$EXTENSION_NAME-arm64" \
        --version-number $VERSION --statement-id "$EXTENSION_NAME-arm64-$VERSION-$region" \
        --principal "*" --action lambda:GetLayerVersion
done
