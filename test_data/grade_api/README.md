# Marcus Debugging Test - Grade Management API

## Overview

This test evaluates Marcus's ability to identify and fix bugs in a complex codebase without being told what the bugs are.

## Files

| File | Description |
|------|-------------|
| `app.py` | The buggy Grade Management API (DO NOT MODIFY ORIGINAL) |
| `BUG_REPORT.md` | Test instructions and expected behavior |
| `run_marcus_test.bat` | Windows test runner script |
| `run_marcus_test.sh` | Linux/Mac test runner script |
| `test_api.py` | API test suite to verify fixes |

## How to Run the Test

### Option 1: Using the Test Script

```bash
# Windows
cd test_data\grade_api
run_marcus_test.bat

# Linux/Mac
cd test_data/grade_api
bash run_marcus_test.sh
```

### Option 2: Manual Marcus Command

```bash
# First, make a backup of the original file
cp app.py app.py.original

# Then run Marcus
./marcus.exe edit app.py "Find and fix all bugs in this Grade Management API. Look for validation issues, logic errors, calculation bugs, data integrity problems, and edge cases."
```

## API Endpoints

The API has the following endpoints:

### Students
- `POST /api/students` - Create student
- `GET /api/students` - List students (paginated)
- `GET /api/students/<id>` - Get student
- `PUT /api/students/<id>` - Update student
- `DELETE /api/students/<id>` - Delete student

### Courses
- `POST /api/courses` - Create course
- `GET /api/courses/<id>` - Get course
- `POST /api/courses/<id>/enroll` - Enroll student

### Grades
- `POST /api/grades` - Create grade
- `GET /api/grades/<student_id>/<course_id>` - Get student grades
- `POST /api/grades/bulk` - Bulk upload grades

### Reports
- `GET /api/reports/course/<course_id>` - Course statistics
- `GET /api/reports/transcript/<student_id>` - Student transcript

### Health
- `GET /api/health` - Health check

## Bug Categories

The API contains bugs in these categories:

1. **Validation Bugs** - Missing input validation
2. **Logic Errors** - Wrong comparison operators, missing checks
3. **Off-by-One Errors** - Pagination, indexing issues
4. **Data Integrity** - Orphaned records, cascade deletes
5. **Calculation Errors** - GPA, averages, distributions

## Evaluation

After Marcus completes the debugging:

1. Compare `app.py` changes against the original
2. Run the test suite to verify fixes work
3. Count bugs found and fixed

### Scoring

| Bugs Found | Score | Rating |
|------------|-------|--------|
| 20-24 | Excellent | Production-ready debugging |
| 15-19 | Good | Solid debugging capabilities |
| 10-14 | Fair | Basic debugging, misses edge cases |
| < 10 | Needs Improvement | Significant gaps |

## Running the Test Suite

After Marcus fixes the bugs, verify with:

```bash
pip install flask pytest requests
python test_api.py
```

## Tips for Better Results

1. **Be specific in prompts**: "Find ALL bugs" vs "Fix some bugs"
2. **Mention categories**: List types of bugs to look for
3. **Ask for explanations**: Request reasoning for each fix
4. **Iterate if needed**: Run multiple passes for missed bugs

## Expected Behavior After Fixes

- All endpoints validate input properly
- GPA is calculated on 4.0 scale with credit weighting
- Enrollment respects capacity and prevents duplicates
- Pagination starts at index 0 for page 1
- Deleting students cleans up related data
- Email validation uses proper regex
- Score validation ensures 0-100 range
- Credits validation ensures 1-6 range
