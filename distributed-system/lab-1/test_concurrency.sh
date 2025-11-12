#!/bin/bash

# A script to test the concurrent connection limit of the server.
#
# Usage: ./test_concurrency.sh <your_public_ip>

# Check if an IP address was provided as an argument
if [ -z "$1" ]; then
    echo "❌ Error: Please provide your EC2 Public IP Address as the first argument."
    echo "   Usage: ./test_concurrency.sh <your_public_ip>"
    exit 1
fi

# Get the IP from the first argument
TARGET_IP=$1
# The number of concurrent clients you want to test
CLIENTS_TO_RUN=11

echo "--- Starting Concurrency Test ($CLIENTS_TO_RUN clients) ---"
echo "--- Target Server: http://${TARGET_IP}/index.html ---"
echo ""

for i in $(seq 1 $CLIENTS_TO_RUN)
do
    echo "Spawning client $i..."
    # -w "..." will print the total time after the request completes
    # & runs this command in the "background", so the loop continues immediately
    # -s (silent) hides the progress bar
    # > /dev/null discards the HTML body
    (time curl -s -w "Client $i finished. Total time: %{time_total}s\n" http://${TARGET_IP}/index.html > /dev/null) &
done

echo ""
echo "--- All $CLIENTS_TO_RUN requests have been sent, awaiting completion... ---"
wait
echo "--- Test Complete ---"