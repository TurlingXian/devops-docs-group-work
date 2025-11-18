#!/bin/bash

# simple test script for the server and proxy

# ----- Global config -----
LOG_DIR="$(pwd)"
CURRENT_DATETIME=$(date '+%d%m%Y')


if [ -z "$1" ]; then
    echo "[ERROR][$(date '+%H:%M:%S')] The IP of TCP server was not specified, abort the program!" >> ""${LOG_DIR}/test-run-${CURRENT_DATETIME}.log
    exit 1
fi

SERVER_IP=$1
SERVER_PORT=8080
PROXY_PORT=30080
SERVER_URL="http://${IP}:${SERVER_PORT}"
PROXY_URL="http://${IP}:${PROXY_PORT}"

# better coloring the text
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

FAIL_COUNT=0

assert_test_status(){
    if [ "$1" -eq "$2" ]; then
        echo -e "${GREEN}PASS${NC}: $3 (Expected $1, Got $2)"
    else
        echo -e "${RED}FAIL${NC}: $3 (Expected $1, Got $2)"
        ((FAIL_COUNT++))
    fi
}

# --- Start Tests ---
echo "--- (1/2) 🚀 Testing http_server at $SERVER_URL ---"

# Test 1: GET 200 OK (index.html)
echo "Running: GET 200 (index.html)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" $SERVER_URL/index.html)
assert_status 200 $CODE "GET 200 OK (index.html)"

# Test 2: GET 404 Not Found
echo "Running: GET 404 (nonexistent-file.txt)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" $SERVER_URL/nonexistent-file.txt)
assert_status 404 $CODE "GET 404 Not Found"

# Test 3: GET 400 Bad Extension
echo "Running: GET 400 (test.md)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" $SERVER_URL/test.md)
assert_status 400 $CODE "GET 400 Bad Extension"

# Test 4: GET 400 Directory Traversal
echo "Running: GET 400 (Directory Traversal)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" $SERVER_URL/../../etc/passwd)
assert_status 400 $CODE "GET 400 Directory Traversal"

# Test 5: Method 501 Not Implemented
echo "Running: PUT 501 (Not Implemented)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT $SERVER_URL/index.html)
assert_status 501 $CODE "Method 501 Not Implemented (PUT)"

# Test 6: POST 201 Created
echo "Running: POST 201 (Creating new_file.txt)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST --data "This is an automated test file" $SERVER_URL/automated_test.txt)
assert_status 201 $CODE "POST 201 Created"

# Test 7: GET 200 (Verify POST)
echo "Running: GET 200 (Verifying automated_test.txt)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" $SERVER_URL/automated_test.txt)
assert_status 200 $CODE "GET 200 OK (Verify POST)"
echo ""

# --- Proxy Tests ---
echo "--- (2/2) 🚀 Testing proxy at $PROXY_URL ---"

# Test 8: Proxy GET 200 OK
echo "Running: Proxy GET 200 (http://example.com)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -x $PROXY_URL http://example.com)
assert_status 200 $CODE "Proxy GET 200 OK (example.com)"

# Test 9: Proxy Method 501
echo "Running: Proxy PUT 501 (http://example.com)"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT -x $PROXY_URL http://example.com)
assert_status 501 $CODE "Proxy 501 Not Implemented (PUT)"
echo ""

# --- Summary ---
echo "--- Test Summary ---"
if [ "$FAIL_COUNT" -eq 0 ]; then
    echo -e "${GREEN}✅ All tests passed! Congratulations!${NC}"
else
    echo -e "${RED}❌ $FAIL_COUNT test(s) failed. Please check your server logs.${NC}"
    exit 1
fi