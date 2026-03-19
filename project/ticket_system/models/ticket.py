"""Ticket model for issue tracking."""
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Optional, List


class TicketStatus(Enum):
    """Ticket status enumeration."""
    OPEN = "open"
    IN_PROGRESS = "in_progress"
    WAITING_CUSTOMER = "waiting_customer"
    WAITING_THIRD_PARTY = "waiting_third_party"
    ESCALATED = "escalated"
    RESOLVED = "resolved"
    CLOSED = "closed"


class TicketPriority(Enum):
    """Ticket priority enumeration."""
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


class TicketCategory(Enum):
    """Ticket category enumeration."""
    HARDWARE = "hardware"
    SOFTWARE = "software"
    NETWORK = "network"
    ACCOUNT = "account"
    BILLING = "billing"
    OTHER = "other"


@dataclass
class Ticket:
    """Ticket entity for support tracking."""
    id: str
    title: str
    description: str
    status: TicketStatus = TicketStatus.OPEN
    priority: TicketPriority = TicketPriority.MEDIUM
    category: TicketCategory = TicketCategory.OTHER
    assignee_id: Optional[str] = None
    reporter_id: str = None
    created_at: datetime = field(default_factory=datetime.now)
    updated_at: datetime = field(default_factory=datetime.now)
    due_date: Optional[datetime] = None
    resolved_at: Optional[datetime] = None
    first_response_at: Optional[datetime] = None
    tags: List[str] = field(default_factory=list)
    todo_ids: List[str] = field(default_factory=list)
    knowledge_base_ids: List[str] = field(default_factory=list)
    customer_id: Optional[str] = None
    sla_id: Optional[str] = None
    
    def to_dict(self) -> dict:
        """Serialize ticket to dictionary."""
        return {
            'id': self.id,
            'title': self.title,
            'description': self.description,
            'status': self.status.value,
            'priority': self.priority.value,
            'category': self.category.value,
            'assignee_id': self.assignee_id,
            'reporter_id': self.reporter_id,
            'created_at': self.created_at.isoformat(),
            'updated_at': self.updated_at.isoformat(),
            'due_date': self.due_date.isoformat() if self.due_date else None,
            'resolved_at': self.resolved_at.isoformat() if self.resolved_at else None,
            'first_response_at': self.first_response_at.isoformat() if self.first_response_at else None,
            'tags': self.tags,
            'todo_ids': self.todo_ids,
            'knowledge_base_ids': self.knowledge_base_ids,
            'customer_id': self.customer_id,
            'sla_id': self.sla_id
        }
    
    @classmethod
    def from_dict(cls, data: dict) -> 'Ticket':
        """Deserialize ticket from dictionary."""
        return cls(
            id=data['id'],
            title=data['title'],
            description=data['description'],
            status=TicketStatus(data['status']),
            priority=TicketPriority(data['priority']),
            category=TicketCategory(data.get('category', 'other')),
            assignee_id=data.get('assignee_id'),
            reporter_id=data.get('reporter_id'),
            created_at=datetime.fromisoformat(data['created_at']),
            updated_at=datetime.fromisoformat(data['updated_at']),
            due_date=datetime.fromisoformat(data['due_date']) if data.get('due_date') else None,
            resolved_at=datetime.fromisoformat(data['resolved_at']) if data.get('resolved_at') else None,
            first_response_at=datetime.fromisoformat(data['first_response_at']) if data.get('first_response_at') else None,
            tags=data.get('tags', []),
            todo_ids=data.get('todo_ids', []),
            knowledge_base_ids=data.get('knowledge_base_ids', []),
            customer_id=data.get('customer_id'),
            sla_id=data.get('sla_id')
        )
