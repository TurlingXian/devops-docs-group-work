#!/bin/bash

LOG_DIR=$(pwd)
CURRENT_DATETIME=$(date '+%d%m%Y')

if [ -z "$1" ]; then
    echo "[ERROR][$(date '+%H:%M:%S')] The IP of TCP server was not specified, abort the program!" >> "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"
    exit 1
fi

SERVER_IP=$1
SERVER_PORT=8080
PROXY_PORT=30080
SERVER_URL="http://${SERVER_IP}:${SERVER_PORT}"
PROXY_URL="http://${SERVER_IP}:${PROXY_PORT}"

# better coloring the text
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

FAIL_COUNT=0

# helper function to quickly evaluate the test case
assert_test_status(){
    # this function expect 2 arguments: test_code and expected_code
    # also require a "helper message" at $3
    # also provide the deposit destination for all message
    # "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"
    if [ "$1" -eq "$2" ]; then
        echo -e "${GREEN}PASS${NC}: $3 (Expected $1, Got $2)" >> "$4"
    else
        echo -e "${RED}FAIL${NC}: $3 (Expected $1, Got $2)" >> "$4"
        ((FAIL_COUNT++))
    fi
}

echo "[$(date '+%H:%M:%S')] Test basic function for the server... $SERVER_URL, port $SERVER_PORT" >> "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"


echo "[$(date '+%H:%M:%S')][BASIC] Function test..." >> "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"
function=$(curl -s -o /dev/null -w "%{http_code}" $SERVER_URL/index.html)
assert_test_status 200 $function "GET 200 OK (index.html)" "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"

function=$(curl -s -o /dev/null -w "%{http_code}" $SERVER_URL/nonexistent-file.txt)
assert_test_status 404 $function "GET 404 Not Found" "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"

echo "[$(date '+%H:%M:%S')] Test ended with $FAIL_COUNT falied" >> "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"
echo "========================================================" >> "${LOG_DIR}/test-run-${CURRENT_DATETIME}.log"