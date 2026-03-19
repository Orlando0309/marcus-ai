from fastapi import APIRouter, HTTPException
from models import Todo
import service

router = APIRouter()


@router.get("/")
def root():
    return {"message": "Todo API is running"}


@router.get("/todos")
def list_todos():
    return {"todos": service.list_todos()}


@router.get("/todos/{todo_id}")
def get_todo(todo_id: str):
    todo = service.get_todo(todo_id)
    if todo is None:
        raise HTTPException(status_code=404, detail="Todo not found")
    return todo


@router.post("/todos")
def create_todo(todo: Todo):
    try:
        return service.create_todo(todo)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.put("/todos/{todo_id}")
def update_todo(todo_id: str, todo: Todo):
    updated = service.update_todo(todo_id, todo)
    if updated is None:
        raise HTTPException(status_code=404, detail="Todo not found")
    return updated


@router.delete("/todos/{todo_id}")
def delete_todo(todo_id: str):
    if not service.delete_todo(todo_id):
        raise HTTPException(status_code=404, detail="Todo not found")
    return {"message": "Todo deleted"}
