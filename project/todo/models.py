from pydantic import BaseModel
from typing import Optional


class Todo(BaseModel):
    id: Optional[str] = None
    title: str
    description: Optional[str] = None
    completed: bool = False
