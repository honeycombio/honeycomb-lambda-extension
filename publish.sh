#!/bin/bash

if [[ "${CIRCLE_TAG}" == *dev ]]; then
    EXTENSION_NAME="honeycomb-lambda-extension-dev"
else
    EXTENSION_NAME="honeycomb-lambda-extension"
fi

REGIONS=(eu-north-1 ap-south-1 eu-west-3 eu-west-2 eu-west-1 ap-northeast-2 ap-northeast-1
         sa-east-1 ca-central-1 ap-southeast-1 ap-southeast-2 eu-central-1 us-east-1
         us-east-2 us-west-1 us-west-2)

if [ ! -f ~/artifacts/extensions/honeycomb-lambda-extension-amd64 ]; then
    echo "amd64 extension does not exist, cannot publish."
    exit 1;
fi

if [ ! -f ~/artifacts/extensions/honeycomb-lambda-extension-arm64 ]; then
    echo "arm64 extension does not exist, cannot publish."
    exit 1;
fi

cd ~/artifacts

mkdir ext-amd64
cp extensions/honeycomb-lambda-extension-amd64 ext-amd64/
zip -r ext-amd64.zip ext-amd64

for region in ${REGIONS[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name "$EXTENSION_NAME-x86_64" \
        --compatible-architectures x86_64 \
        --region $region --zip-file "fileb://ext-amd64.zip"`
    VERSION=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name "$EXTENSION_NAME-x86_64" \
        --version-number $VERSION --statement-id "$EXTENSION_NAME-x86_64-$VERSION-$region" \
        --principal "*" --action lambda:GetLayerVersion
done

mkdir ext-arm64
cp extensions/honeycomb-lambda-extension-arm64 ext-arm64/
zip -r ext-arm64.zip ext-arm64

for region in ${REGIONS[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name "$EXTENSION_NAME-arm64" \
        --compatible-architectures arm64 \
        --region $region --zip-file "fileb://ext-arm64.zip"`
    VERSION=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name "$EXTENSION_NAME-arm64" \
        --version-number $VERSION --statement-id "$EXTENSION_NAME-arm64-$VERSION-$region" \
        --principal "*" --action lambda:GetLayerVersion
done
