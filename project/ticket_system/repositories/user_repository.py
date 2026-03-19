"""User repository for user data persistence."""
from typing import Optional, List
from .base import BaseRepository
from ..models import User


class UserRepository(BaseRepository[User]):
    """Repository for User entities."""
    
    @property
    def entity_name(self) -> str:
        return "user"
    
    def _deserialize(self, data: dict) -> User:
        return User.from_dict(data)
    
    def _serialize(self, entity: User) -> dict:
        return entity.to_dict()
    
    def find_by_username(self, username: str) -> Optional[User]:
        """Find user by username."""
        for user in self._cache.values():
            if user.username == username:
                return user
        return None
    
    def find_by_email(self, email: str) -> Optional[User]:
        """Find user by email."""
        for user in self._cache.values():
            if user.email == email:
                return user
        return None
    
    def find_active_users(self) -> List[User]:
        """Get all active users."""
        return [u for u in self._cache.values() if u.is_active]
    
    def find_by_role(self, role: str) -> List[User]:
        """Find users by role."""
        return [u for u in self._cache.values() if u.role == role]
    
    def find_by_department(self, department: str) -> List[User]:
        """Find users by department."""
        return [u for u in self._cache.values() if u.department == department]
    
    def find_by_skill(self, skill: str) -> List[User]:
        """Find users with a specific skill."""
        return [u for u in self._cache.values() if skill in u.skills]
