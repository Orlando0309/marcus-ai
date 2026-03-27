"""
Test Suite for Grade Management API
Run this AFTER Marcus has fixed the bugs to verify the fixes work correctly.
"""

import pytest
import sys
import os

# Add parent directory to path to import app
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from app import app, students_db, courses_db, enrollments_db, grades_db


@pytest.fixture
def client():
    """Create a test client"""
    app.config['TESTING'] = True
    with app.test_client() as client:
        # Clear all databases before each test
        students_db.clear()
        courses_db.clear()
        enrollments_db.clear()
        grades_db.clear()
        yield client


@pytest.fixture
def sample_student():
    return {
        'name': 'John Doe',
        'email': 'john@example.com',
        'student_id': 'STU001'
    }


@pytest.fixture
def sample_course():
    return {
        'title': 'Computer Science 101',
        'code': 'CS101',
        'credits': 3,
        'max_students': 30
    }


# ============== VALIDATION TESTS ==============

class TestStudentValidation:
    """Test student endpoint validation"""

    def test_create_student_invalid_email(self, client):
        """Should reject invalid email formats"""
        invalid_emails = [
            'notanemail',
            'missing@domain',
            '@nodomain.com',
            'spaces @email.com',
            ''
        ]

        for email in invalid_emails:
            response = client.post('/api/students', json={
                'name': 'Test User',
                'email': email
            })
            assert response.status_code == 400, f"Should reject invalid email: {email}"

    def test_create_student_valid_email(self, client, sample_student):
        """Should accept valid email"""
        response = client.post('/api/students', json=sample_student)
        assert response.status_code == 201
        data = response.get_json()
        assert len(data['id']) == 36  # Full UUID length


class TestCourseValidation:
    """Test course endpoint validation"""

    def test_create_course_invalid_credits(self, client, sample_course):
        """Should reject credits outside 1-6 range"""
        invalid_credits = [0, -1, 7, 10, 100]

        for credits_val in invalid_credits:
            course = sample_course.copy()
            course['credits'] = credits_val
            response = client.post('/api/courses', json=course)
            assert response.status_code == 400, f"Should reject credits: {credits_val}"

    def test_create_course_valid_credits(self, client, sample_course):
        """Should accept credits in 1-6 range"""
        for credits_val in [1, 2, 3, 4, 5, 6]:
            course = sample_course.copy()
            course['credits'] = credits_val
            response = client.post('/api/courses', json=course)
            assert response.status_code == 201

    def test_create_course_invalid_max_students(self, client, sample_course):
        """Should reject non-positive max_students"""
        for max_val in [0, -1, -100]:
            course = sample_course.copy()
            course['max_students'] = max_val
            response = client.post('/api/courses', json=course)
            assert response.status_code == 400, f"Should reject max_students: {max_val}"


class TestGradeValidation:
    """Test grade endpoint validation"""

    def test_create_grade_invalid_score(self, client, sample_student, sample_course):
        """Should reject scores outside 0-100 range"""
        # Create student and course first
        student_resp = client.post('/api/students', json=sample_student)
        student_id = student_resp.get_json()['id']

        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        # Enroll student
        client.post(f'/api/courses/{course_id}/enroll', json={'student_id': student_id})

        invalid_scores = [-1, -100, 101, 150, 1000]

        for score in invalid_scores:
            response = client.post('/api/grades', json={
                'student_id': student_id,
                'course_id': course_id,
                'score': score
            })
            assert response.status_code == 400, f"Should reject score: {score}"


# ============== LOGIC TESTS ==============

class TestEnrollmentLogic:
    """Test enrollment logic"""

    def test_enroll_when_course_full(self, client, sample_student, sample_course):
        """Should reject enrollment when course is full"""
        # Create course with max 2 students
        sample_course['max_students'] = 2
        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        # Create and enroll 2 students
        for i in range(2):
            student = {'name': f'Student {i}', 'email': f'stu{i}@example.com'}
            student_resp = client.post('/api/students', json=student)
            student_id = student_resp.get_json()['id']

            enroll_resp = client.post(f'/api/courses/{course_id}/enroll',
                                      json={'student_id': student_id})
            assert enroll_resp.status_code == 201

        # Try to enroll a 3rd student
        student3 = {'name': 'Student 3', 'email': 'stu3@example.com'}
        student3_resp = client.post('/api/students', json=student3)
        student3_id = student3_resp.get_json()['id']

        enroll_resp = client.post(f'/api/courses/{course_id}/enroll',
                                  json={'student_id': student3_id})
        assert enroll_resp.status_code == 400
        assert 'full' in enroll_resp.get_json()['error'].lower()

    def test_enroll_already_enrolled(self, client, sample_student, sample_course):
        """Should reject duplicate enrollment"""
        # Create student and course
        student_resp = client.post('/api/students', json=sample_student)
        student_id = student_resp.get_json()['id']

        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        # Enroll first time
        enroll_resp1 = client.post(f'/api/courses/{course_id}/enroll',
                                   json={'student_id': student_id})
        assert enroll_resp1.status_code == 201

        # Try to enroll again
        enroll_resp2 = client.post(f'/api/courses/{course_id}/enroll',
                                   json={'student_id': student_id})
        assert enroll_resp2.status_code == 400

    def test_enroll_non_existent_student(self, client, sample_course):
        """Should reject enrollment of non-existent student"""
        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        enroll_resp = client.post(f'/api/courses/{course_id}/enroll',
                                  json={'student_id': 'non-existent-id'})
        assert enroll_resp.status_code == 404


class TestPaginationLogic:
    """Test pagination logic"""

    def test_pagination_page_one(self, client, sample_student):
        """Page 1 should return first per_page items"""
        # Create 15 students
        for i in range(15):
            student = {'name': f'Student {i}', 'email': f'stu{i}@example.com'}
            client.post('/api/students', json=student)

        # Get page 1 with per_page=10
        response = client.get('/api/students?page=1&per_page=10')
        data = response.get_json()

        assert len(data['students']) == 10
        # First student should be at index 0
        assert data['students'][0]['name'] == 'Student 0'

    def test_pagination_page_two(self, client, sample_student):
        """Page 2 should return next per_page items"""
        # Create 15 students
        for i in range(15):
            student = {'name': f'Student {i}', 'email': f'stu{i}@example.com'}
            client.post('/api/students', json=student)

        # Get page 2 with per_page=10
        response = client.get('/api/students?page=2&per_page=10')
        data = response.get_json()

        assert len(data['students']) == 5
        # First student on page 2 should be Student 10
        assert data['students'][0]['name'] == 'Student 10'


class TestDataIntegrity:
    """Test data integrity"""

    def test_delete_student_cascades(self, client, sample_student, sample_course):
        """Deleting a student should clean up enrollments and grades"""
        # Create student and course
        student_resp = client.post('/api/students', json=sample_student)
        student_id = student_resp.get_json()['id']

        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        # Enroll and add grade
        client.post(f'/api/courses/{course_id}/enroll', json={'student_id': student_id})
        client.post('/api/grades', json={
            'student_id': student_id,
            'course_id': course_id,
            'score': 85
        })

        # Delete student
        delete_resp = client.delete(f'/api/students/{student_id}')
        assert delete_resp.status_code == 204

        # Verify student is deleted
        get_resp = client.get(f'/api/students/{student_id}')
        assert get_resp.status_code == 404


class TestGradeEnrollmentCheck:
    """Test that grades require enrollment"""

    def test_grade_non_enrolled_student(self, client, sample_student, sample_course):
        """Should reject grade for non-enrolled student"""
        # Create student and course but DON'T enroll
        student_resp = client.post('/api/students', json=sample_student)
        student_id = student_resp.get_json()['id']

        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        # Try to add grade
        grade_resp = client.post('/api/grades', json={
            'student_id': student_id,
            'course_id': course_id,
            'score': 85
        })
        assert grade_resp.status_code == 400


# ============== CALCULATION TESTS ==============

class TestGPACalculation:
    """Test GPA calculations"""

    def test_gpa_on_4_0_scale(self, client, sample_student, sample_course):
        """GPA should be on 4.0 scale, not average of scores"""
        # Create student and course
        student_resp = client.post('/api/students', json=sample_student)
        student_id = student_resp.get_json()['id']

        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        # Enroll
        client.post(f'/api/courses/{course_id}/enroll', json={'student_id': student_id})

        # Add grade with score 90 (should be A = 4.0)
        client.post('/api/grades', json={
            'student_id': student_id,
            'course_id': course_id,
            'score': 90
        })

        # Get transcript
        transcript = client.get(f'/api/reports/transcript/{student_id}')
        data = transcript.get_json()

        # GPA should be on 4.0 scale, not 90
        # Score 90 = A- = 3.7 in +/- grading system
        assert data['gpa'] <= 4.0, f"GPA should be on 4.0 scale, got {data['gpa']}"
        assert data['gpa'] == 3.7  # A- for score 90

    def test_gpa_weighted_by_credits(self, client, sample_student):
        """GPA should be weighted by course credits"""
        student_resp = client.post('/api/students', json=sample_student)
        student_id = student_resp.get_json()['id']

        # Create two courses with different credits
        course1 = {'title': 'Course 1', 'code': 'C1', 'credits': 4}
        course2 = {'title': 'Course 2', 'code': 'C2', 'credits': 2}

        c1_resp = client.post('/api/courses', json=course1)
        c1_id = c1_resp.get_json()['id']

        c2_resp = client.post('/api/courses', json=course2)
        c2_id = c2_resp.get_json()['id']

        # Enroll in both
        client.post(f'/api/courses/{c1_id}/enroll', json={'student_id': student_id})
        client.post(f'/api/courses/{c2_id}/enroll', json={'student_id': student_id})

        # A in 4-credit course (16 quality points), F in 2-credit course (0 quality points)
        client.post('/api/grades', json={'student_id': student_id, 'course_id': c1_id, 'score': 90})
        client.post('/api/grades', json={'student_id': student_id, 'course_id': c2_id, 'score': 50})

        transcript = client.get(f'/api/reports/transcript/{student_id}')
        data = transcript.get_json()

        # Weighted GPA: score 90 = A- (3.7), score 50 = F (0.0)
        # (4*3.7 + 2*0.0) / 6 = 14.8/6 = 2.47
        expected_gpa = (4 * 3.7 + 2 * 0.0) / 6
        assert abs(data['gpa'] - expected_gpa) < 0.01


class TestLetterGrades:
    """Test letter grade calculation"""

    def test_letter_grade_boundaries(self):
        """Test letter grade boundaries"""
        from app import calculate_letter_grade

        # Test boundaries
        assert calculate_letter_grade(100) in ['A', 'A+', 'A']
        assert calculate_letter_grade(90) in ['A', 'A-', 'A']
        assert calculate_letter_grade(89) in ['B', 'B+', 'B']
        assert calculate_letter_grade(80) in ['B', 'B-', 'B']
        assert calculate_letter_grade(79) in ['C', 'C+', 'C']
        assert calculate_letter_grade(70) in ['C', 'C-', 'C']
        assert calculate_letter_grade(69) in ['D', 'D+', 'D']
        assert calculate_letter_grade(60) in ['D', 'D-', 'D']
        assert calculate_letter_grade(59) in ['F']
        assert calculate_letter_grade(0) in ['F']


class TestCourseReport:
    """Test course report statistics"""

    def test_distribution_accuracy(self, client, sample_student, sample_course):
        """Grade distribution should be accurate"""
        course_resp = client.post('/api/courses', json=sample_course)
        course_id = course_resp.get_json()['id']

        # Create students and enroll
        student_ids = []
        for i in range(5):
            student = {'name': f'Student {i}', 'email': f'stu{i}@example.com'}
            s_resp = client.post('/api/students', json=student)
            student_ids.append(s_resp.get_json()['id'])
            client.post(f'/api/courses/{course_id}/enroll', json={'student_id': student_ids[-1]})

        # Add grades: 2 A's, 2 B's, 1 C
        scores = [95, 92, 85, 82, 75]
        for sid, score in zip(student_ids, scores):
            client.post('/api/grades', json={
                'student_id': sid,
                'course_id': course_id,
                'score': score
            })

        # Get report
        report = client.get(f'/api/reports/course/{course_id}')
        data = report.get_json()

        stats = data['statistics']
        assert stats['total_students'] == 5
        assert stats['highest'] == 95
        assert stats['lowest'] == 75
        assert abs(stats['average'] - 85.8) < 0.01


# ============== RUN TESTS ==============

if __name__ == '__main__':
    pytest.main([__file__, '-v', '--tb=short'])
