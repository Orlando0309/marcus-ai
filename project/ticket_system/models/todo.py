"""Todo model for checklist items within tickets."""
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Optional


class TodoStatus(Enum):
    """Todo status enumeration."""
    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"


@dataclass
class Todo:
    """Todo entity for checklist items."""
    id: str
    title: str
    ticket_id: str
    status: TodoStatus = TodoStatus.PENDING
    order: int = 0
    created_at: datetime = field(default_factory=datetime.now)
    completed_at: Optional[datetime] = None
    assignee_id: Optional[str] = None
    
    def to_dict(self) -> dict:
        """Serialize todo to dictionary."""
        return {
            'id': self.id,
            'title': self.title,
            'ticket_id': self.ticket_id,
            'status': self.status.value,
            'order': self.order,
            'created_at': self.created_at.isoformat(),
            'completed_at': self.completed_at.isoformat() if self.completed_at else None,
            'assignee_id': self.assignee_id
        }
    
    @classmethod
    def from_dict(cls, data: dict) -> 'Todo':
        """Deserialize todo from dictionary."""
        return cls(
            id=data['id'],
            title=data['title'],
            ticket_id=data['ticket_id'],
            status=TodoStatus(data['status']),
            order=data.get('order', 0),
            created_at=datetime.fromisoformat(data['created_at']),
            completed_at=datetime.fromisoformat(data['completed_at']) if data.get('completed_at') else None,
            assignee_id=data.get('assignee_id')
        )
