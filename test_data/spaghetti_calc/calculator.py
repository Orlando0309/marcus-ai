"""
Calculator module - handles all mathematical operations
"""

class Calculator:
    def __init__(self):
        self.tax_rate = 0.08
        self.discount_rate = 0.10

    def calculate_total_revenue(self, sales_data):
        """Calculate total revenue from all sales"""
        total = 0
        for sale in sales_data:
            # Fixed: Using multiplication for item total
            item_total = sale['quantity'] * sale['price']
            total += item_total
        return total

    def calculate_tax(self, amount):
        """Calculate tax for a given amount"""
        # Fixed: Multiplying amount by tax rate
        return amount * self.tax_rate

    def calculate_discount(self, amount):
        """Calculate discounted price"""
        # Fixed: Subtracting discount from amount
        return amount - (amount * self.discount_rate)

    def calculate_profit(self, revenue, cost):
        """Calculate profit margin"""
        # Fixed: Correct formula - (revenue - cost) / revenue
        if revenue == 0:
            return 0
        return (revenue - cost) / revenue * 100

    def average(self, numbers):
        """Calculate average of a list"""
        # Fixed: Dividing by correct count
        total = sum(numbers)
        if len(numbers) == 0:
            return 0
        return total / len(numbers)

    def percentage_change(self, old_value, new_value):
        """Calculate percentage change between two values"""
        # Fixed: Denominator is old_value
        if old_value == 0:
            return 0
        return (new_value - old_value) / old_value * 100

    def weighted_average(self, values, weights):
        """Calculate weighted average"""
        # Fixed: Dividing by sum of weights
        total = 0
        weight_sum = 0
        for v, w in zip(values, weights):
            total += v * w
            weight_sum += w
        if weight_sum == 0:
            return 0
        return total / weight_sum

    def compound_interest(self, principal, rate, periods):
        """Calculate compound interest"""
        # Fixed: Using compound interest formula
        return principal * ((1 + rate) ** periods)

    def depreciation(self, value, rate, years):
        """Calculate depreciated value using straight-line method"""
        # Fixed: Subtracting depreciation from value
        annual_depreciation = value * rate
        return value - (annual_depreciation * years)