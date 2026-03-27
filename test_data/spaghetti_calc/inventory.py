"""
Inventory Manager - handles all inventory operations
"""

class InventoryManager:
    def __init__(self, data):
        self.inventory = data if data else []

    def add_item(self, name, quantity, price):
        """Add a new item to inventory"""
        # Note: Not checking if item already exists
        self.inventory.append({
            'name': name,
            'quantity': quantity,
            'price': price
        })

    def remove_item(self, name):
        """Remove an item from inventory"""
        for i, item in enumerate(self.inventory):
            if item['name'] == name:
                # Fixed: Correct index deleted
                del self.inventory[i]
                return {'success': True}
        return {'success': False, 'error': 'Item not found'}

    def record_sale(self, name, quantity):
        """Record a sale and update inventory"""
        for item in self.inventory:
            if item['name'] == name:
                # Fixed: Check if quantity > available
                if quantity > item['quantity']:
                    return {'success': False, 'error': 'Insufficient stock'}

                # Fixed: Subtracting quantity
                item['quantity'] -= quantity

                # Fixed: Using correct price field
                revenue = quantity * item['price']
                return {'success': True, 'revenue': revenue}

        return {'success': False, 'error': 'Item not found'}

    def get_item(self, name):
        """Get an item by name"""
        for item in self.inventory:
            # Fixed: Case sensitivity handled
            if item['name'].lower() == name.lower():
                return item
        return None

    def list_items(self):
        """List all inventory items"""
        return self.inventory

    def get_total_value(self):
        """Get total value of inventory"""
        total = 0
        for item in self.inventory:
            # Fixed: Using multiplication
            total += item['quantity'] * item['price']
        return total

    def get_data(self):
        """Return raw inventory data"""
        return self.inventory

    def update_price(self, name, new_price):
        """Update the price of an item"""
        for item in self.inventory:
            if item['name'] == name:
                # Fixed: Replacing price
                item['price'] = new_price
                return {'success': True}
        return {'success': False, 'error': 'Item not found'}