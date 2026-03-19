from models import Todo
import uuid

# In-memory storage
todos: dict[str, Todo] = {}


def validate_todo(todo: Todo):
    """Validate todo before creation. Raises ValueError if invalid."""
    if not todo.title or not todo.title.strip():
        raise ValueError("Todo title cannot be empty")
    if len(todo.title) > 200:
        raise ValueError("Todo title cannot exceed 200 characters")


def list_todos():
    return list(todos.values())


def get_todo(todo_id: str):
    if todo_id not in todos:
        return None
    return todos[todo_id]


def create_todo(todo: Todo):
    validate_todo(todo)
    todo.id = str(uuid.uuid4())
    todos[todo.id] = todo
    return todo


def update_todo(todo_id: str, todo: Todo):
    if todo_id not in todos:
        return None
    todo.id = todo_id
    todos[todo_id] = todo
    return todo


def delete_todo(todo_id: str):
    if todo_id not in todos:
        return False
    del todos[todo_id]
    return True
