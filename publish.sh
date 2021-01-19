#!/bin/bash

if [[ -z "${CIRCLE_TAG}" ]]; then
    EXTENSION_NAME="honeycomb-lambda-extension-dev"
else
    EXTENSION_NAME="honeycomb-lambda-extension"
fi

REGIONS=(eu-north1 ap-south-1 eu-west-3 eu-west-2 eu-west-1 ap-northeast-2 ap-northeast-1
         sa-east-1 ca-central-1 ap-southeast-1 ap-southeast-2 eu-central-1 us-east-1
         us-east-2 us-west-1 us-west-2)

if [ ! -f ~/artifacts/extensions/honeycomb-lambda-extension ]; then
    echo "extension does not exist, cannot publish."
    exit 1;
fi

cd ~/artifacts
zip -r extension.zip extensions

for region in ${REGIONS[@]}; do
    RESPONSE=`aws lambda publish-layer-version \
        --layer-name $EXTENSION_NAME \
        --region $region --zip-file "fileb://extension.zip"`
    VERSION=`echo $RESPONSE | jq -r '.Version'`
    aws --region $region lambda add-layer-version-permission --layer-name $EXTENSION_NAME \
        --version-number $VERSION --statement-id "$EXTENSION_NAME-$VERSION-$region" \
        --principal "*" --action lambda:GetLayerVersion
done
