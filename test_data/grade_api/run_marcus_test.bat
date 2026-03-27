@echo off
REM Marcus Debug Test Script - Windows Version
REM Tests Marcus ability to find and fix bugs in a Grade Management API

echo ==============================================
echo   MARCUS DEBUGGING TEST
echo   Grade Management API
echo ==============================================
echo.

set SCRIPT_DIR=%~dp0
set APP_FILE=%SCRIPT_DIR%app.py
set MARCUS_EXE=%SCRIPT_DIR%..\..\marcus.exe
set LOG_FILE=%SCRIPT_DIR%marcus_test_log.txt

echo Step 1: Checking prerequisites...
echo -------------------------------------------

if not exist "%APP_FILE%" (
    echo ERROR: app.py not found at %APP_FILE%
    exit /b 1
)

if not exist "%MARCUS_EXE%" (
    echo Marcus not found. Building...
    cd %SCRIPT_DIR%..\..
    go build -o marcus.exe .\cmd\marcus
    if errorlevel 1 (
        echo ERROR: Failed to build marcus.exe
        exit /b 1
    )
    echo Build successful!
)

echo.
echo Step 2: Test Information
echo -------------------------------------------
echo Target file: %APP_FILE%
echo Marcus executable: %MARCUS_EXE%
echo Log file: %LOG_FILE%
echo.

for /f "tokens=*" %%a in ('wc -l ^< "%APP_FILE%"') do set ORIGINAL_LINES=%%a
echo Original file has %ORIGINAL_LINES% lines
echo.

echo Step 3: Running Marcus Debug Test
echo ==============================================
echo.
echo INSTRUCTION TO MARCUS:
echo -------------------------------------------
echo I need you to debug this Grade Management REST API ^(app.py^).
echo.
echo The API handles students, courses, enrollments, grades, and reports.
echo.
echo Please:
echo 1. Carefully review the entire file
echo 2. Identify ALL bugs ^(logic errors, validation issues, calculation errors, data integrity problems, edge cases^)
echo 3. Fix each bug you find
echo 4. Explain what you found and why
echo.
echo The code has multiple types of bugs:
echo - Missing input validation
echo - Logic errors in conditionals
echo - Calculation errors ^(GPA, averages, pagination^)
echo - Data integrity issues
echo - Edge case handling problems
echo.
echo Find and fix as many bugs as you can. Be thorough!
echo -------------------------------------------
echo.

cd %SCRIPT_DIR%..\..

set INSTRUCTION=Review this Grade Management API and find ALL bugs. Look for: missing validation, logic errors, calculation bugs, data integrity issues, and edge cases. Fix each bug and explain what you found.

echo Executing Marcus edit command...
echo.

REM Run Marcus edit command
%MARCUS_EXE% edit "%APP_FILE%" "%INSTRUCTION%" 2>&1 | tee "%LOG_FILE%"

echo.
echo ==============================================
echo Step 4: Test Complete
echo ==============================================
echo.
echo Log saved to: %LOG_FILE%
echo.
echo ==============================================
echo NEXT STEPS
echo ==============================================
echo.
echo 1. Review the changes Marcus made
echo 2. Compare against BUG_REPORT.md for expected bugs found
echo 3. Count how many bugs Marcus identified and fixed
echo.
echo Scoring:
echo   20-24 bugs: Excellent
echo   15-19 bugs: Good
echo   10-14 bugs: Fair
echo   ^< 10 bugs:   Needs Improvement
echo.
