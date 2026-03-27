"""
Grade Management REST API
A Flask-based API for managing students, courses, and grades.
Fixed version - all bugs corrected.
"""

from flask import Flask, request, jsonify
from datetime import datetime
import uuid
import re

app = Flask(__name__)

# Email validation regex pattern
EMAIL_REGEX = re.compile(r'^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$')

# In-memory storage
students_db = {}
courses_db = {}
enrollments_db = {}
grades_db = {}


# ============== STUDENT ENDPOINTS ==============

@app.route('/api/students', methods=['POST'])
def create_student():
    """Create a new student"""
    data = request.get_json()

    if not data:
        return jsonify({'error': 'Request body required'}), 400

    name = data.get('name')
    email = data.get('email')
    student_id = data.get('student_id')

    # Fix Bug 1: Validate email format
    if not name or not email:
        return jsonify({'error': 'Name and email required'}), 400

    if not EMAIL_REGEX.match(email):
        return jsonify({'error': 'Invalid email format'}), 400

    # Fix Bug 2: Use full UUID
    student_uuid = str(uuid.uuid4())

    student = {
        'id': student_uuid,
        'name': name,
        'email': email,
        'student_id': student_id,
        'created_at': datetime.now().isoformat(),
        'gpa': 0.0
    }

    students_db[student_uuid] = student

    return jsonify(student), 201


@app.route('/api/students/<student_id>', methods=['GET'])
def get_student(student_id):
    """Get a student by ID"""
    # Fix Bug 3: Handle case-insensitive lookup properly
    student = students_db.get(student_id) or students_db.get(student_id.lower())

    if not student:
        return jsonify({'error': 'Student not found'}), 404

    return jsonify(student)


@app.route('/api/students/<student_id>', methods=['PUT'])
def update_student(student_id):
    """Update student information"""
    student = students_db.get(student_id)

    if not student:
        return jsonify({'error': 'Student not found'}), 404

    data = request.get_json()

    if data.get('name'):
        student['name'] = data['name']

    if data.get('email'):
        # Fix Bug 4: Validate new email format
        if not EMAIL_REGEX.match(data['email']):
            return jsonify({'error': 'Invalid email format'}), 400
        student['email'] = data['email']

    if data.get('student_id'):
        student['student_id'] = data['student_id']

    student['updated_at'] = datetime.now().isoformat()

    return jsonify(student)


@app.route('/api/students/<student_id>', methods=['DELETE'])
def delete_student(student_id):
    """Delete a student"""
    if student_id not in students_db:
        return jsonify({'error': 'Student not found'}), 404

    # Fix Bug 5: Cascade delete - remove related enrollments and grades
    enrollment_ids_to_delete = []
    for enf_id, enrollment in enrollments_db.items():
        if enrollment['student_id'] == student_id:
            enrollment_ids_to_delete.append(enf_id)
    for enf_id in enrollment_ids_to_delete:
        del enrollments_db[enf_id]

    # Remove related grades
    grade_keys_to_delete = []
    for gkey in grades_db.keys():
        if gkey.startswith(f"{student_id}_"):
            grade_keys_to_delete.append(gkey)
    for gkey in grade_keys_to_delete:
        del grades_db[gkey]

    del students_db[student_id]

    return '', 204


@app.route('/api/students', methods=['GET'])
def list_students():
    """List all students with optional pagination"""
    page = request.args.get('page', 1)
    per_page = request.args.get('per_page', 10)

    # Fix Bug 6: Correct pagination calculation
    page_num = max(1, int(page))
    per_page_num = max(1, int(per_page))
    start_idx = (page_num - 1) * per_page_num
    end_idx = start_idx + per_page_num

    all_students = list(students_db.values())
    paginated = all_students[start_idx:end_idx]

    return jsonify({
        'students': paginated,
        'total': len(all_students),
        'page': page_num,
        'per_page': per_page_num
    })


# ============== COURSE ENDPOINTS ==============

@app.route('/api/courses', methods=['POST'])
def create_course():
    """Create a new course"""
    data = request.get_json()

    if not data:
        return jsonify({'error': 'Request body required'}), 400

    title = data.get('title')
    code = data.get('code')
    credits = data.get('credits')
    max_students = data.get('max_students', 30)

    # Fix Bug 7: Validate credits range (1-6)
    if credits is None or not isinstance(credits, (int, float)) or credits < 1 or credits > 6:
        return jsonify({'error': 'Credits must be between 1 and 6'}), 400

    # Fix Bug 8: Validate max_students is positive
    if max_students is None or not isinstance(max_students, int) or max_students <= 0:
        return jsonify({'error': 'max_students must be a positive integer'}), 400

    if not title or not code:
        return jsonify({'error': 'Title and code required'}), 400

    course_uuid = str(uuid.uuid4())

    course = {
        'id': course_uuid,
        'title': title,
        'code': code,
        'credits': credits,
        'max_students': max_students,
        'enrolled_count': 0,
        'created_at': datetime.now().isoformat()
    }

    courses_db[course_uuid] = course

    return jsonify(course), 201


@app.route('/api/courses/<course_id>', methods=['GET'])
def get_course(course_id):
    """Get a course by ID"""
    course = courses_db.get(course_id)

    if not course:
        return jsonify({'error': 'Course not found'}), 404

    return jsonify(course)


@app.route('/api/courses/<course_id>/enroll', methods=['POST'])
def enroll_student(course_id):
    """Enroll a student in a course"""
    data = request.get_json()
    student_id = data.get('student_id')

    if not student_id:
        return jsonify({'error': 'Student ID required'}), 400

    course = courses_db.get(course_id)
    if not course:
        return jsonify({'error': 'Course not found'}), 404

    student = students_db.get(student_id)
    if not student:
        return jsonify({'error': 'Student not found'}), 404

    # Fix Bug 9: Correct comparison operator (>= instead of <)
    if course['enrolled_count'] >= course['max_students']:
        return jsonify({'error': 'Course is full'}), 400

    # Fix Bug 10: Check if student already enrolled
    for enrollment in enrollments_db.values():
        if enrollment['student_id'] == student_id and enrollment['course_id'] == course_id:
            return jsonify({'error': 'Student already enrolled in this course'}), 400

    enrollment_id = str(uuid.uuid4())

    enrollment = {
        'id': enrollment_id,
        'course_id': course_id,
        'student_id': student_id,
        'enrolled_at': datetime.now().isoformat()
    }

    enrollments_db[enrollment_id] = enrollment
    course['enrolled_count'] += 1

    return jsonify(enrollment), 201


# ============== GRADE ENDPOINTS ==============

@app.route('/api/grades', methods=['POST'])
def create_grade():
    """Create or update a grade for a student in a course"""
    data = request.get_json()

    if not data:
        return jsonify({'error': 'Request body required'}), 400

    student_id = data.get('student_id')
    course_id = data.get('course_id')
    score = data.get('score')
    grade_type = data.get('grade_type', 'assignment')

    # Fix Bug 11: Validate score range (0-100)
    if score is None or not isinstance(score, (int, float)) or score < 0 or score > 100:
        return jsonify({'error': 'Score must be between 0 and 100'}), 400

    if not student_id or not course_id or score is None:
        return jsonify({'error': 'Student ID, Course ID, and score required'}), 400

    # Fix Bug 12: Check if student is enrolled in course
    is_enrolled = False
    for enrollment in enrollments_db.values():
        if enrollment['student_id'] == student_id and enrollment['course_id'] == course_id:
            is_enrolled = True
            break
    if not is_enrolled:
        return jsonify({'error': 'Student not enrolled in this course'}), 400

    # Validate grade_type
    valid_types = ['assignment', 'quiz', 'exam', 'project', 'final']
    if grade_type not in valid_types:
        return jsonify({'error': f'Invalid grade type. Must be one of: {valid_types}'}), 400

    grade_key = f"{student_id}_{course_id}_{grade_type}"

    grade = {
        'id': str(uuid.uuid4()),
        'student_id': student_id,
        'course_id': course_id,
        'score': score,
        'grade_type': grade_type,
        'letter_grade': calculate_letter_grade(score),
        'created_at': datetime.now().isoformat()
    }

    grades_db[grade_key] = grade

    # Update student GPA
    update_student_gpa(student_id)

    return jsonify(grade), 201


def calculate_letter_grade(score):
    """Convert numeric score to letter grade"""
    # Fix Bug 13: Add +/- grades with proper thresholds
    if score >= 90:
        if score >= 97:
            return 'A+'
        elif score >= 93:
            return 'A'
        else:
            return 'A-'
    elif score >= 80:
        if score >= 87:
            return 'B+'
        elif score >= 83:
            return 'B'
        else:
            return 'B-'
    elif score >= 70:
        if score >= 77:
            return 'C+'
        elif score >= 73:
            return 'C'
        else:
            return 'C-'
    elif score >= 60:
        if score >= 67:
            return 'D+'
        elif score >= 63:
            return 'D'
        else:
            return 'D-'
    else:
        return 'F'


@app.route('/api/grades/<student_id>/<course_id>', methods=['GET'])
def get_student_grades(student_id, course_id):
    """Get all grades for a student in a course"""
    student_grades = []

    for key, grade in grades_db.items():
        if grade['student_id'] == student_id and grade['course_id'] == course_id:
            student_grades.append(grade)

    return jsonify({
        'student_id': student_id,
        'course_id': course_id,
        'grades': student_grades,
        'average': calculate_average_grade(student_grades)
    })


def calculate_average_grade(grades):
    """Calculate average score from list of grades"""
    if not grades:
        return 0.0

    total = 0
    count = 0

    for grade in grades:
        total += grade['score']
        count += 1

    # Fix Bug 14 & 15: Safe division with explicit check
    return total / count if count > 0 else 0.0


def update_student_gpa(student_id):
    """Update a student's GPA based on all their grades"""
    student = students_db.get(student_id)
    if not student:
        return

    student_grades = []
    for key, grade in grades_db.items():
        if grade['student_id'] == student_id:
            student_grades.append(grade)

    if not student_grades:
        student['gpa'] = 0.0
        return

    # Fix Bug 16: Calculate GPA on 4.0 scale using quality points and credits
    total_quality_points = 0
    total_credits = 0

    for grade in student_grades:
        course = courses_db.get(grade['course_id'])
        if course:
            credits = course.get('credits', 3)
            total_quality_points += score_to_quality_points(grade['score']) * credits
            total_credits += credits

    student['gpa'] = round(total_quality_points / total_credits, 2) if total_credits > 0 else 0.0


# ============== REPORTING ENDPOINTS ==============

@app.route('/api/reports/course/<course_id>', methods=['GET'])
def get_course_report(course_id):
    """Generate a report for a course"""
    course = courses_db.get(course_id)
    if not course:
        return jsonify({'error': 'Course not found'}), 404

    course_grades = []
    for key, grade in grades_db.items():
        if grade['course_id'] == course_id:
            course_grades.append(grade)

    if not course_grades:
        return jsonify({
            'course': course,
            'statistics': {
                'average': 0,
                'highest': 0,
                'lowest': 0,
                'distribution': {}
            }
        })

    scores = [g['score'] for g in course_grades]

    # Fix Bug 17 & 18: Handle all grade types including +/-
    distribution = {
        'A+': 0, 'A': 0, 'A-': 0,
        'B+': 0, 'B': 0, 'B-': 0,
        'C+': 0, 'C': 0, 'C-': 0,
        'D+': 0, 'D': 0, 'D-': 0,
        'F': 0
    }

    for score in scores:
        letter = calculate_letter_grade(score)
        if letter in distribution:
            distribution[letter] += 1

    return jsonify({
        'course': course,
        'statistics': {
            'average': sum(scores) / len(scores),
            'highest': max(scores),
            'lowest': min(scores),
            'distribution': distribution,
            'total_students': len(scores)
        }
    })


@app.route('/api/reports/transcript/<student_id>', methods=['GET'])
def get_transcript(student_id):
    """Generate a transcript for a student"""
    student = students_db.get(student_id)
    if not student:
        return jsonify({'error': 'Student not found'}), 404

    student_grades = []
    total_credits = 0
    quality_points = 0

    for key, grade in grades_db.items():
        if grade['student_id'] == student_id:
            course = courses_db.get(grade['course_id'])
            if course:
                student_grades.append({
                    'course': course,
                    'grade': grade
                })
                # Fix Bug 19 & 20: Account for credits in quality points
                credits = course.get('credits', 3)
                quality_points += score_to_quality_points(grade['score']) * credits
                total_credits += course.get('credits', 3)

    # Fix Bug 21: Correct GPA calculation
    gpa = round(quality_points / total_credits, 2) if total_credits > 0 else 0.0

    return jsonify({
        'student': student,
        'grades': student_grades,
        'gpa': gpa,
        'total_credits': total_credits
    })


def score_to_quality_points(score):
    """Convert score to quality points for GPA calculation"""
    # Fix Bug 22: Add +/- grade conversions
    letter = calculate_letter_grade(score)
    conversion = {
        'A+': 4.0, 'A': 4.0, 'A-': 3.7,
        'B+': 3.3, 'B': 3.0, 'B-': 2.7,
        'C+': 2.3, 'C': 2.0, 'C-': 1.7,
        'D+': 1.3, 'D': 1.0, 'D-': 0.7,
        'F': 0.0
    }
    return conversion.get(letter, 0.0)


# ============== BULK OPERATIONS ==============

@app.route('/api/grades/bulk', methods=['POST'])
def bulk_upload_grades():
    """Bulk upload grades from a list"""
    data = request.get_json()

    if not data or 'grades' not in data:
        return jsonify({'error': 'Grades array required'}), 400

    results = {
        'successful': [],
        'failed': []
    }

    for grade_data in data['grades']:
        try:
            student_id = grade_data.get('student_id')
            course_id = grade_data.get('course_id')
            score = grade_data.get('score')

            # Fix Bug 23: Validate each grade entry properly
            if score is None or not isinstance(score, (int, float)) or score < 0 or score > 100:
                results['failed'].append({
                    'data': grade_data,
                    'error': 'Score must be between 0 and 100'
                })
                continue

            if not student_id or not course_id:
                results['failed'].append({
                    'data': grade_data,
                    'error': 'Missing student_id or course_id'
                })
                continue

            # Create the grade
            grade_key = f"{student_id}_{course_id}_assignment"
            grade = {
                'id': str(uuid.uuid4()),
                'student_id': student_id,
                'course_id': course_id,
                'score': score,
                'grade_type': 'assignment',
                'letter_grade': calculate_letter_grade(score),
                'created_at': datetime.now().isoformat()
            }
            grades_db[grade_key] = grade
            results['successful'].append(grade)

        except Exception as e:
            results['failed'].append({
                'data': grade_data,
                'error': str(e)
            })

    return jsonify(results), 200


# ============== HEALTH CHECK ==============

@app.route('/api/health', methods=['GET'])
def health_check():
    """Health check endpoint"""
    return jsonify({
        'status': 'healthy',
        'timestamp': datetime.now().isoformat(),
        'stats': {
            'students': len(students_db),
            'courses': len(courses_db),
            'enrollments': len(enrollments_db),
            'grades': len(grades_db)
        }
    })


if __name__ == '__main__':
    app.run(debug=True, port=5000)
