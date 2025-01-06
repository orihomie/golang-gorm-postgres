#!/usr/bin/env bash

BASE_URL=http://localhost:8888/api/v1
TELEGRAM_USER_ID=${1:-6794234746}
PASSWORD=${2:-h5sh3d}
PROFILE_ID=${3:-7e852ab6-878f-4f5b-84d2-2ee2749bfcee}

access_token=$(curl -s -X POST ${BASE_URL}/auth/bot/login \
                    -H "Content-Type: application/json" \
                    -d "{\"telegramUserId\": \"${TELEGRAM_USER_ID}\", \"password\": \"${PASSWORD}\"}" | jq -r .access_token)

echo $access_token

curl_cmd="curl -X POST ${BASE_URL}/profiles/ -H \"Authorization: Bearer $access_token\""

eval $curl_cmd