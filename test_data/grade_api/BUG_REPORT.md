# Grade Management API - Test Instructions

This document describes the testing task for Marcus.

## Objective

Debug the `app.py` file - a Grade Management REST API built with Flask.

## API Overview

The API manages:
- **Students** - Create, read, update, delete students
- **Courses** - Create courses and enroll students
- **Grades** - Record and calculate grades
- **Reports** - Generate course reports and student transcripts

## Expected Behavior

### Students
- Email should be validated (proper email format)
- Pagination should work correctly (page 1 starts at index 0)
- Deleting a student should clean up related data
- Student lookup should be consistent (case handling)

### Courses
- Credits should be between 1-6
- Max students should be positive
- Enrollment should respect capacity limits
- Students shouldn't be enrolled twice in same course

### Grades
- Scores must be between 0-100
- Only enrolled students should receive grades
- Letter grades should follow standard scale
- GPA should be on a 4.0 scale

### Reports
- Course reports should show accurate statistics
- Transcripts should calculate GPA correctly using credits

## Your Task

1. **Read** the `app.py` file carefully
2. **Identify** all bugs (logic errors, validation issues, calculation errors, edge cases)
3. **Fix** each bug you find
4. **Document** what you fixed and why

## Success Criteria

- All input validation is properly implemented
- All calculations (GPA, averages, distributions) are mathematically correct
- No data integrity issues (orphaned records, etc.)
- Logic flows are correct (enrollment, pagination, etc.)
- Edge cases are handled properly

## How to Run

After fixing, the API should be testable with:

```bash
pip install flask
python app.py
```

Then test endpoints with curl or Postman.

---

**Good luck! Find and fix as many bugs as you can.**
