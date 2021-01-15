#!/bin/bash

if [[ -z "${CIRCLE_TAG}" ]]; then
	EXTENSION_NAME="honeycomb-lambda-extension-dev"
else
	EXTENSION_NAME="honeycomb-lambda-extension"
fi

REGIONS=`aws ec2 describe-regions | jq -r '.Regions[].RegionName'`

if [ ! -f ~/artifacts/extensions/honeycomb-lambda-extension ]; then
    echo "extension does not exist, cannot publish."
    exit 1;
fi

cd ~/artifacts
zip -r extension.zip extensions

for region in $REGIONS; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name $EXTENSION_NAME \
        --region $region --zip-file "fileb://extension.zip"`
    VERSION=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name $EXTENSION_NAME \
        --version-number $VERSION --statement-id "honeycombLambdaExtensionDev-$VERSION-$region" \
        --principal "*" --action lambda:GetLayerVersion
done
