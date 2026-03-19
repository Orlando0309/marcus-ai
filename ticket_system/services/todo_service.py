"""Todo service for checklist management."""
from typing import List, Optional
from datetime import datetime
from ..repositories import TodoRepository, TicketRepository
from ..factories import TodoFactory
from ..models import Todo, TodoStatus


class TodoService:
    """Service for todo/checklist management.
    
    Follows the Service Layer pattern to orchestrate repository operations.
    """
    
    def __init__(self, todo_repo: TodoRepository, ticket_repo: TicketRepository):
        self.todo_repo = todo_repo
        self.ticket_repo = ticket_repo
    
    def create_todo(self, title: str, ticket_id: str, 
                    assignee_id: Optional[str] = None) -> Todo:
        """Create a new todo item for a ticket."""
        ticket = self.ticket_repo.find_by_id(ticket_id)
        if not ticket:
            raise ValueError("Ticket not found")
        
        # Get the next order number
        existing_todos = self.todo_repo.find_by_ticket(ticket_id)
        order = len(existing_todos)
        
        todo = TodoFactory.create(
            title=title,
            ticket_id=ticket_id,
            order=order,
            assignee_id=assignee_id
        )
        todo = self.todo_repo.save(todo)
        
        # Update ticket with todo reference
        ticket.todo_ids.append(todo.id)
        self.ticket_repo.save(ticket)
        
        return todo
    
    def get_todo(self, todo_id: str) -> Optional[Todo]:
        """Get a todo by ID."""
        return self.todo_repo.find_by_id(todo_id)
    
    def get_todos_for_ticket(self, ticket_id: str) -> List[Todo]:
        """Get all todos for a ticket."""
        return self.todo_repo.find_by_ticket(ticket_id)
    
    def get_todos_by_status(self, status: TodoStatus) -> List[Todo]:
        """Get todos by status."""
        return self.todo_repo.find_by_status(status)
    
    def start_todo(self, todo_id: str) -> Todo:
        """Mark todo as in progress."""
        todo = self.todo_repo.find_by_id(todo_id)
        if not todo:
            raise ValueError("Todo not found")
        
        todo.status = TodoStatus.IN_PROGRESS
        return self.todo_repo.save(todo)
    
    def complete_todo(self, todo_id: str) -> Todo:
        """Mark todo as completed."""
        todo = self.todo_repo.find_by_id(todo_id)
        if not todo:
            raise ValueError("Todo not found")
        
        todo.status = TodoStatus.COMPLETED
        todo.completed_at = datetime.now()
        return self.todo_repo.save(todo)
    
    def uncomplete_todo(self, todo_id: str) -> Todo:
        """Mark todo as pending again."""
        todo = self.todo_repo.find_by_id(todo_id)
        if not todo:
            raise ValueError("Todo not found")
        
        todo.status = TodoStatus.PENDING
        todo.completed_at = None
        return self.todo_repo.save(todo)
    
    def update_title(self, todo_id: str, title: str) -> Todo:
        """Update todo title."""
        todo = self.todo_repo.find_by_id(todo_id)
        if not todo:
            raise ValueError("Todo not found")
        
        todo.title = title
        return self.todo_repo.save(todo)
    
    def assign_todo(self, todo_id: str, assignee_id: str) -> Todo:
        """Assign todo to a user."""
        todo = self.todo_repo.find_by_id(todo_id)
        if not todo:
            raise ValueError("Todo not found")
        
        todo.assignee_id = assignee_id
        return self.todo_repo.save(todo)
    
    def reorder_todos(self, ticket_id: str, todo_ids: List[str]) -> List[Todo]:
        """Reorder todos within a ticket."""
        todos = []
        for order, todo_id in enumerate(todo_ids):
            todo = self.todo_repo.find_by_id(todo_id)
            if todo and todo.ticket_id == ticket_id:
                todo.order = order
                self.todo_repo.save(todo)
                todos.append(todo)
        return todos
    
    def delete_todo(self, todo_id: str) -> bool:
        """Delete a todo."""
        todo = self.todo_repo.find_by_id(todo_id)
        if not todo:
            return False
        
        # Remove from ticket
        ticket = self.ticket_repo.find_by_id(todo.ticket_id)
        if ticket and todo_id in ticket.todo_ids:
            ticket.todo_ids.remove(todo_id)
            self.ticket_repo.save(ticket)
        
        return self.todo_repo.delete(todo_id)
    
    def get_completion_percentage(self, ticket_id: str) -> float:
        """Get completion percentage for a ticket's todos."""
        todos = self.get_todos_for_ticket(ticket_id)
        if not todos:
            return 0.0
        
        completed = sum(1 for t in todos if t.status == TodoStatus.COMPLETED)
        return (completed / len(todos)) * 100
