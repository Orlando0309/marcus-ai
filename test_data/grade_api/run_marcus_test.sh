#!/bin/bash

# Marcus Debug Test Script
# Tests Marcus ability to find and fix bugs in a Grade Management API

set -e

echo "=============================================="
echo "  MARCUS DEBUGGING TEST"
echo "  Grade Management API"
echo "=============================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_FILE="$SCRIPT_DIR/app.py"
MARCUS_EXE="$SCRIPT_DIR/../../marcus.exe"
FIXED_FILE="$SCRIPT_DIR/app_fixed.py"
LOG_FILE="$SCRIPT_DIR/marcus_test_log.txt"

# Check prerequisites
echo "Step 1: Checking prerequisites..."
echo "-------------------------------------------"

if [ ! -f "$APP_FILE" ]; then
    echo -e "${RED}ERROR: app.py not found at $APP_FILE${NC}"
    exit 1
fi

if [ ! -f "$MARCUS_EXE" ]; then
    echo -e "${YELLOW}WARNING: marcus.exe not found. Building...${NC}"
    cd "$SCRIPT_DIR/../../"
    go build -o marcus.exe ./cmd/marcus
    if [ $? -ne 0 ]; then
        echo -e "${RED}ERROR: Failed to build marcus.exe${NC}"
        exit 1
    fi
    echo -e "${GREEN}Build successful!${NC}"
fi

echo ""

# Display test info
echo "Step 2: Test Information"
echo "-------------------------------------------"
echo "Target file: $APP_FILE"
echo "Marcus executable: $MARCUS_EXE"
echo "Log file: $LOG_FILE"
echo ""

# Count original lines
ORIGINAL_LINES=$(wc -l < "$APP_FILE")
echo "Original file has $ORIGINAL_LINES lines"
echo ""

# Start the test
echo "Step 3: Running Marcus Debug Test"
echo "=============================================="
echo ""
echo "INSTRUCTION TO MARCUS:"
echo "-------------------------------------------"
cat << 'PROMPT'

I need you to debug this Grade Management REST API (app.py).

The API handles students, courses, enrollments, grades, and reports.

Please:
1. Carefully review the entire file
2. Identify ALL bugs (logic errors, validation issues, calculation errors, data integrity problems, edge cases)
3. Fix each bug you find
4. Explain what you found and why it's a bug

The code has multiple types of bugs:
- Missing input validation
- Logic errors in conditionals
- Calculation errors (GPA, averages, pagination)
- Data integrity issues
- Edge case handling problems

Find and fix as many bugs as you can. Be thorough!
PROMPT

echo ""
echo "-------------------------------------------"
echo ""

# Run Marcus edit command
echo "Executing: marcus edit app.py with debug instruction..."
echo ""

cd "$SCRIPT_DIR/.."

# Create the instruction
INSTRUCTION="Review this Grade Management API and find ALL bugs. Look for: missing validation, logic errors, calculation bugs, data integrity issues, and edge cases. Fix each bug and explain what you found."

# Run Marcus in non-interactive mode
# Note: This assumes marcus edit can work with auto-confirm or we pipe 'y'
echo "$INSTRUCTION" | $MARCUS_EXE edit "$APP_FILE" "$INSTRUCTION" 2>&1 | tee "$LOG_FILE"

echo ""
echo "=============================================="
echo "Step 4: Test Complete"
echo "=============================================="
echo ""

# Count new lines
if [ -f "$APP_FILE" ]; then
    NEW_LINES=$(wc -l < "$APP_FILE")
    echo "Fixed file has $NEW_LINES lines"
fi

echo ""
echo "Log saved to: $LOG_FILE"
echo ""

# Summary
echo "=============================================="
echo "NEXT STEPS"
echo "=============================================="
echo ""
echo "1. Review the changes Marcus made"
echo "2. Compare against BUG_REPORT.md for expected bugs"
echo "3. Count how many bugs Marcus found"
echo ""
echo "Scoring:"
echo "  20-24 bugs: Excellent"
echo "  15-19 bugs: Good"
echo "  10-14 bugs: Fair"
echo "  < 10 bugs:   Needs Improvement"
echo ""
