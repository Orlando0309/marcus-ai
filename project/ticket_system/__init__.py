# Ticket Management System
# Using Repository and Factory Design Patterns

from .models import User, Ticket, Todo, TicketStatus, TicketPriority
from .repositories import UserRepository, TicketRepository, TodoRepository
from .factories import UserFactory, TicketFactory, TodoFactory
from .services import AuthService, TicketService, TodoService
