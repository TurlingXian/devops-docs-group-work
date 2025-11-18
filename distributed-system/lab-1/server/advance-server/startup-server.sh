#!/bin/bash

# global variables
LOG_DIR="$HOME"
CURRENT_DATETIME=$(date '+%d%m%Y')
SERVER_PATH=

# check the program-id, assume that the program name is "advance-server"
program_pid=$(ps -ef | grep advance-server | grep -v grep | tr -s ' ' | cut -d ' ' -f2)

if [ -n "${program_pid}" ]
then
    echo "Not Null, find success with pid: $program_pid"  >> "${LOG_DIR}/server-startup-${CURRENT_DATETIME}.log"
else
    echo "[$(date '+%H:%M:%S')]Program is either not started or not found!" >> "${LOG_DIR}/error-${CURRENT_DATETIME}.log"
    # start the server
    "$HOME/server/advance-server/advance-server" 8080
fi



