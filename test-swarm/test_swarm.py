#!/usr/bin/env python3
"""
Test Swarm Feature - Complex Python Project Generator

This test creates a complex Python project that requires multi-agent collaboration:
- A full-featured REST API with authentication, database models, migrations
- Background task processing with Redis/Celery
- Real-time WebSocket support
- Comprehensive test suite
- Docker containerization
- CI/CD configuration

This complexity demonstrates why swarm (multi-agent) execution is valuable:
- Architect agent: Designs system structure and interfaces
- Coder agent: Implements the actual code
- Reviewer agent: Reviews for bugs and best practices
- Debugger agent: Fixes issues found during testing
"""

import os
import sys
from pathlib import Path

# Test configuration
TEST_PROJECT_NAME = "complex_python_api"
TEST_DIR = Path(__file__).parent / TEST_PROJECT_NAME

def create_test_project_structure():
    """Create a complex Python project structure that needs swarm."""

    print("=" * 60)
    print("SWARM TEST: Complex Python API Project")
    print("=" * 60)
    print()
    print("This test creates a complex Python project requiring:")
    print("  - Architect: Design system structure & interfaces")
    print("  - Coder: Implement code")
    print("  - Reviewer: Code review & quality checks")
    print("  - Debugger: Fix issues")
    print()

    # Project structure
    structure = {
        "app": {
            "__init__.py": "",
            "main.py": "# FastAPI application entry point\n",
            "config.py": "# Application configuration\n",
            "database.py": "# Database connection & session management\n",
            "models": {
                "__init__.py": "",
                "user.py": "# User model\n",
                "post.py": "# Post model\n",
                "comment.py": "# Comment model\n",
            },
            "schemas": {
                "__init__.py": "",
                "user.py": "# Pydantic schemas for user\n",
                "post.py": "# Pydantic schemas for post\n",
                "token.py": "# Token schemas\n",
            },
            "api": {
                "__init__.py": "",
                "v1": {
                    "__init__.py": "",
                    "routes": {
                        "__init__.py": "",
                        "users.py": "# User routes\n",
                        "posts.py": "# Post routes\n",
                        "auth.py": "# Authentication routes\n",
                    },
                },
            },
            "services": {
                "__init__.py": "",
                "auth.py": "# Authentication service\n",
                "email.py": "# Email service\n",
                "cache.py": "# Redis cache service\n",
            },
            "tasks": {
                "__init__.py": "",
                "celery_app.py": "# Celery configuration\n",
                "worker.py": "# Background tasks\n",
            },
            "websocket": {
                "__init__.py": "",
                "manager.py": "# WebSocket connection manager\n",
                "routes.py": "# WebSocket routes\n",
            },
            "middleware": {
                "__init__.py": "",
                "auth.py": "# Authentication middleware\n",
                "logging.py": "# Request logging middleware\n",
            },
            "utils": {
                "__init__.py": "",
                "security.py": "# Password hashing, JWT\n",
                "validators.py": "# Input validators\n",
            },
        },
        "tests": {
            "__init__.py": "",
            "conftest.py": "# Pytest fixtures\n",
            "test_users.py": "# User tests\n",
            "test_posts.py": "# Post tests\n",
            "test_auth.py": "# Authentication tests\n",
            "test_websocket.py": "# WebSocket tests\n",
        },
        "alembic": {
            "versions": "",
        },
        "docker": {
            "Dockerfile": "# Application Dockerfile\n",
            "docker-compose.yml": "# Docker Compose configuration\n",
        },
        ".github": {
            "workflows": {
                "ci.yml": "# CI/CD pipeline\n",
            },
        },
        ".env.example": "# Environment variables template\n",
        "requirements.txt": "# Python dependencies\n",
        "requirements-dev.txt": "# Development dependencies\n",
        "pyproject.toml": "# Project configuration\n",
        "README.md": "# Project documentation\n",
        "Makefile": "# Common commands\n",
    }

    print("Creating project structure...")
    create_structure(TEST_DIR, structure)

    print(f"\nCreated {TEST_DIR}")
    print("\nProject structure:")
    print_tree(TEST_DIR)

    return TEST_DIR


def create_structure(base_path: Path, structure: dict):
    """Recursively create directory structure."""
    for name, content in structure.items():
        path = base_path / name
        if isinstance(content, dict):
            # It's a directory
            path.mkdir(parents=True, exist_ok=True)
            create_structure(path, content)
        else:
            # It's a file
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)


def print_tree(path: Path, prefix: str = ""):
    """Print directory tree."""
    items = sorted(path.iterdir())
    for i, item in enumerate(items):
        is_last = i == len(items) - 1
        connector = "+-- " if is_last else "|-- "
        print(f"{prefix}{connector}{item.name}")
        if item.is_dir():
            extension = "    " if is_last else "|   "
            print_tree(item, prefix + extension)


def generate_swarm_test_goal():
    """Generate the goal that would trigger swarm execution."""

    goal = """
Implement a production-ready REST API for a blog platform with the following requirements:

1. **Authentication & Authorization**
   - JWT-based authentication with refresh tokens
   - Role-based access control (admin, author, reader)
   - Password reset flow with email verification
   - OAuth2 social login (Google, GitHub)

2. **Core Features**
   - User management (CRUD, profile updates)
   - Blog posts with rich text content
   - Comments with threading
   - Tags and categories
   - Full-text search with Elasticsearch

3. **Real-time Features**
   - WebSocket notifications for new comments
   - Live post view count
   - Real-time collaboration indicators

4. **Background Processing**
   - Email notifications via Celery
   - Image processing and thumbnail generation
   - Scheduled content publishing
   - Analytics aggregation

5. **Infrastructure**
   - PostgreSQL database with Alembic migrations
   - Redis for caching and Celery broker
   - Docker containerization
   - CI/CD with GitHub Actions
   - Comprehensive test suite (80%+ coverage)

6. **Security**
   - Rate limiting
   - CORS configuration
   - Input validation and sanitization
   - SQL injection prevention
   - XSS protection

This is a complex multi-component system requiring careful architecture design,
clean implementation, thorough review, and debugging of integration issues.
"""

    return goal.strip()


def run_test():
    """Run the swarm test."""

    print("\n" + "=" * 60)
    print("TEST SCENARIO")
    print("=" * 60)
    print()

    goal = generate_swarm_test_goal()
    print("Goal to be executed by swarm:")
    print("-" * 60)
    print(goal[:500] + "...")
    print("-" * 60)
    print()

    # Create the project structure
    create_test_project_structure()

    print("\n" + "=" * 60)
    print("SWARM EXECUTION SIMULATION")
    print("=" * 60)
    print()
    print("Recommended agent composition for this goal:")
    print("  1. Architect - Design system architecture")
    print("  2. Coder - Implement the code")
    print("  3. Reviewer - Review for quality/issues")
    print("  4. Debugger - Fix any issues found")
    print()
    print("Execution phases:")
    print("  Phase 1: Planning & Design (Architect leads)")
    print("  Phase 2: Implementation (Coder leads)")
    print("  Phase 3: Code Review (Reviewer leads)")
    print("  Phase 4: Testing & Debugging (Debugger leads)")
    print()

    # Test outcome tracking
    print("=" * 60)
    print("OUTCOME TRACKING TEST")
    print("=" * 60)
    print()
    print("Simulating action outcomes...")

    outcomes = [
        ("write_file", True, "Created app/main.py"),
        ("write_file", True, "Created app/models/user.py"),
        ("write_file", False, "Failed: app/services/auth.py - import error"),
        ("run_command", True, "Ran pytest - 45 passed"),
        ("run_command", False, "Failed: docker-compose up - port conflict"),
        ("read_file", True, "Read app/config.py"),
        ("search_code", True, "Found all JWT usages"),
    ]

    for action_type, success, description in outcomes:
        status = "[OK]" if success else "[FAIL]"
        print(f"  {status} {action_type}: {description}")

    print()
    print("Outcome statistics would be recorded for future confidence scoring.")
    print()

    # Test confidence assessment
    print("=" * 60)
    print("CONFIDENCE ASSESSMENT TEST")
    print("=" * 60)
    print()

    confidence_factors = {
        "Historical success rate": "75% (based on similar tasks)",
        "Context completeness": "85% (requirements clear)",
        "Task complexity": "High (multiple components)",
        "Tool availability": "100% (all tools available)",
        "Time pressure": "Low (no deadline)",
        "Ambiguity": "Medium (some requirements vague)",
    }

    print("Confidence factors:")
    for factor, value in confidence_factors.items():
        print(f"  - {factor}: {value}")

    print()
    print("Estimated confidence: 72% (MEDIUM)")
    print("Recommendation: PROCEED (but verify critical decisions)")
    print()

    # Test capability registry
    print("=" * 60)
    print("CAPABILITY REGISTRY TEST")
    print("=" * 60)
    print()

    capabilities = {
        "fastapi": "expert (95%)",
        "sqlalchemy": "proficient (85%)",
        "celery": "proficient (80%)",
        "websocket": "intermediate (65%)",
        "elasticsearch": "intermediate (60%)",
        "docker": "proficient (82%)",
    }

    print("Registered capabilities:")
    for cap, level in capabilities.items():
        print(f"  - {cap}: {level}")

    print()
    print("Self-assessment:")
    print("  Strengths: FastAPI, SQLAlchemy, Docker")
    print("  Weaknesses: Elasticsearch, WebSocket scaling")
    print("  Requires human help: Production Elasticsearch tuning")
    print()

    print("=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print()
    print("This complex Python project demonstrates why swarm execution is valuable:")
    print()
    print("  1. Single-agent execution would:")
    print("     - Miss architectural considerations")
    print("     - Lack diverse code review perspectives")
    print("     - Take longer without parallel specialization")
    print()
    print("  2. Swarm execution provides:")
    print("     - Specialized agents for each phase")
    print("     - Shared blackboard for knowledge")
    print("     - Inter-agent communication for coordination")
    print("     - Outcome tracking for continuous improvement")
    print()

    return True


if __name__ == "__main__":
    success = run_test()
    sys.exit(0 if success else 1)
