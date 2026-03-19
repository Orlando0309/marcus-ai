"""User model for authentication and ownership."""
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, List
import hashlib
import uuid


@dataclass
class User:
    """User entity for the support system."""
    id: str
    username: str
    email: str
    password_hash: str
    role: str = "user"  # user, agent, admin
    created_at: datetime = field(default_factory=datetime.now)
    last_login: Optional[datetime] = None
    is_active: bool = True
    department: Optional[str] = None
    skills: List[str] = field(default_factory=list)
    
    @staticmethod
    def hash_password(password: str) -> str:
        """Hash a password using SHA-256."""
        return hashlib.sha256(password.encode()).hexdigest()
    
    def verify_password(self, password: str) -> bool:
        """Verify a password against the stored hash."""
        return self.password_hash == self.hash_password(password)
    
    def to_dict(self) -> dict:
        """Serialize user to dictionary."""
        return {
            'id': self.id,
            'username': self.username,
            'email': self.email,
            'password_hash': self.password_hash,
            'role': self.role,
            'created_at': self.created_at.isoformat(),
            'last_login': self.last_login.isoformat() if self.last_login else None,
            'is_active': self.is_active,
            'department': self.department,
            'skills': self.skills
        }
    
    @classmethod
    def from_dict(cls, data: dict) -> 'User':
        """Deserialize user from dictionary."""
        return cls(
            id=data['id'],
            username=data['username'],
            email=data['email'],
            password_hash=data['password_hash'],
            role=data.get('role', 'user'),
            created_at=datetime.fromisoformat(data['created_at']),
            last_login=datetime.fromisoformat(data['last_login']) if data.get('last_login') else None,
            is_active=data.get('is_active', True),
            department=data.get('department'),
            skills=data.get('skills', [])
        )
