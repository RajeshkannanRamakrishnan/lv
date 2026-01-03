#!/bin/bash
# Generate a 100MB log file
for i in {1..500000}; do
    echo "2025-01-01 12:00:00 [INFO] This is log line $i with some random text to make it longer and simulate a real log entry." >> large.log
done
