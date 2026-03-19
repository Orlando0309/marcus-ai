"""Base repository using Repository Pattern."""
from abc import ABC, abstractmethod
from typing import TypeVar, Generic, List, Optional
import json
from pathlib import Path

T = TypeVar('T')


class BaseRepository(ABC, Generic[T]):
    """Abstract base repository following the Repository Pattern."""
    
    def __init__(self, storage_path: str = "project/data"):
        self.storage_path = Path(storage_path)
        self.storage_path.mkdir(parents=True, exist_ok=True)
        self._cache: dict = {}
        self._load_all()
    
    @property
    @abstractmethod
    def entity_name(self) -> str:
        """Return the entity name for file storage."""
        pass
    
    @abstractmethod
    def _deserialize(self, data: dict) -> T:
        """Deserialize dictionary to entity."""
        pass
    
    @abstractmethod
    def _serialize(self, entity: T) -> dict:
        """Serialize entity to dictionary."""
        pass
    
    @property
    def file_path(self) -> Path:
        """Get the storage file path."""
        return self.storage_path / f"{self.entity_name}s.json"
    
    def _load_all(self) -> None:
        """Load all entities from storage."""
        if self.file_path.exists():
            with open(self.file_path, 'r', encoding='utf-8') as f:
                data = json.load(f)
                for item in data:
                    entity = self._deserialize(item)
                    self._cache[entity.id] = entity
    
    def _save_all(self) -> None:
        """Persist all entities to storage."""
        data = [self._serialize(e) for e in self._cache.values()]
        with open(self.file_path, 'w', encoding='utf-8') as f:
            json.dump(data, f, indent=2)
    
    def find_by_id(self, id: str) -> Optional[T]:
        """Find entity by ID."""
        return self._cache.get(id)
    
    def find_all(self) -> List[T]:
        """Get all entities."""
        return list(self._cache.values())
    
    def save(self, entity: T) -> T:
        """Create or update an entity."""
        self._cache[entity.id] = entity
        self._save_all()
        return entity
    
    def delete(self, id: str) -> bool:
        """Delete entity by ID."""
        if id in self._cache:
            del self._cache[id]
            self._save_all()
            return True
        return False
    
    def find_by(self, **criteria) -> List[T]:
        """Find entities matching criteria."""
        results = []
        for entity in self._cache.values():
            match = True
            for key, value in criteria.items():
                if getattr(entity, key, None) != value:
                    match = False
                    break
            if match:
                results.append(entity)
        return results
