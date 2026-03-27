"""
Test suite for the Inventory Management System
Tests all calculations and business logic
"""

import pytest
import sys
import os

# Add parent directory to path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from calculator import Calculator
from inventory import InventoryManager
from reports import ReportGenerator
from data_loader import load_data


class TestCalculator:
    @pytest.fixture
    def calc(self):
        return Calculator()

    def test_total_revenue(self, calc):
        """Revenue should be quantity * price summed"""
        sales = [
            {'quantity': 10, 'price': 25.00},
            {'quantity': 5, 'price': 35.00}
        ]
        result = calc.calculate_total_revenue(sales)
        # Expected: 10*25 + 5*35 = 250 + 175 = 425
        assert result == 425.0, f"Expected 425.0, got {result}"

    def test_tax_calculation(self, calc):
        """Tax should be amount * tax_rate"""
        result = calc.calculate_tax(100.0)
        # Expected: 100 * 0.08 = 8.0
        assert result == 8.0, f"Expected 8.0, got {result}"

    def test_discount_calculation(self, calc):
        """Discount should reduce the amount"""
        result = calc.calculate_discount(100.0)
        # Expected: 100 - (100 * 0.10) = 90.0
        assert result == 90.0, f"Expected 90.0, got {result}"

    def test_profit_margin(self, calc):
        """Profit margin should be (revenue - cost) / revenue * 100"""
        result = calc.calculate_profit(100.0, 60.0)
        # Expected: (100 - 60) / 100 * 100 = 40.0%
        assert result == 40.0, f"Expected 40.0, got {result}"

    def test_average(self, calc):
        """Average should be sum / count"""
        result = calc.average([10, 20, 30])
        # Expected: 60 / 3 = 20.0
        assert result == 20.0, f"Expected 20.0, got {result}"

    def test_percentage_change(self, calc):
        """Percentage change should be (new - old) / old * 100"""
        result = calc.percentage_change(50, 75)
        # Expected: (75 - 50) / 50 * 100 = 50.0%
        assert result == 50.0, f"Expected 50.0, got {result}"

    def test_weighted_average(self, calc):
        """Weighted average should divide by sum of weights"""
        result = calc.weighted_average([80, 90], [2, 3])
        # Expected: (80*2 + 90*3) / (2+3) = (160 + 270) / 5 = 86.0
        assert result == 86.0, f"Expected 86.0, got {result}"

    def test_compound_interest(self, calc):
        """Compound interest: P(1+r)^n"""
        result = calc.compound_interest(1000, 0.05, 2)
        # Expected: 1000 * (1.05)^2 = 1102.5
        assert result == 1102.5, f"Expected 1102.5, got {result}"

    def test_depreciation(self, calc):
        """Depreciation should reduce value"""
        result = calc.depreciation(1000, 0.10, 2)
        # Expected: 1000 - (1000 * 0.10 * 2) = 800.0
        assert result == 800.0, f"Expected 800.0, got {result}"


class TestInventoryManager:
    @pytest.fixture
    def inv(self):
        return InventoryManager([
            {'name': 'Widget', 'quantity': 100, 'price': 25.00}
        ])

    def test_add_item(self, inv):
        """Should add new item to inventory"""
        initial_count = len(inv.list_items())
        inv.add_item('Gadget', 50, 35.00)
        assert len(inv.list_items()) == initial_count + 1

    def test_remove_item(self, inv):
        """Should remove item from inventory"""
        inv.add_item('Gadget', 50, 35.00)
        result = inv.remove_item('Gadget')
        assert result['success'] == True
        assert len(inv.list_items()) == 1

    def test_record_sale_decreases_quantity(self, inv):
        """Recording a sale should decrease inventory"""
        initial_qty = inv.list_items()[0]['quantity']
        result = inv.record_sale('Widget', 10)
        assert result['success'] == True
        item = inv.get_item('Widget')
        assert item['quantity'] == initial_qty - 10, f"Expected {initial_qty - 10}, got {item['quantity']}"

    def test_sale_insufficient_stock(self, inv):
        """Should fail if trying to sell more than available"""
        result = inv.record_sale('Widget', 200)
        assert result['success'] == False
        assert 'Insufficient' in result['error']

    def test_get_total_value(self, inv):
        """Total value should be sum of (qty * price)"""
        inv.add_item('Gadget', 50, 35.00)
        result = inv.get_total_value()
        # Expected: 100*25 + 50*35 = 2500 + 1750 = 4250
        assert result == 4250.0, f"Expected 4250.0, got {result}"

    def test_update_price_replaces(self, inv):
        """Updating price should replace, not add"""
        inv.update_price('Widget', 30.00)
        item = inv.get_item('Widget')
        assert item['price'] == 30.00, f"Expected 30.00, got {item['price']}"


class TestReportGenerator:
    @pytest.fixture
    def report_gen(self):
        return ReportGenerator()

    @pytest.fixture
    def inv_mgr(self):
        return InventoryManager([
            {'name': 'Widget', 'quantity': 100, 'price': 25.00},
            {'name': 'Gadget', 'quantity': 5, 'price': 35.00}  # Low stock
        ])

    @pytest.fixture
    def calc(self):
        return Calculator()

    @pytest.fixture
    def sales_data(self):
        return [
            {'item': 'Widget', 'quantity': 10, 'price': 25.00, 'revenue': 250.00},
            {'item': 'Gadget', 'quantity': 5, 'price': 35.00, 'revenue': 175.00}
        ]

    def test_inventory_alerts(self, report_gen, inv_mgr):
        """Should alert for items below threshold"""
        alerts = report_gen.generate_inventory_alerts(inv_mgr, threshold=10)
        assert len(alerts) == 1
        assert 'Gadget' in alerts[0]

    def test_monthly_report_values(self, report_gen, inv_mgr, calc, sales_data):
        """Report should show correct values"""
        report = report_gen.generate_monthly_report(inv_mgr, calc, sales_data)

        # Revenue should be 250 + 175 = 425
        assert '425.00' in report or '425' in report


class TestDataLoader:
    def test_load_nonexistent_returns_empty_list(self):
        """Loading non-existent file should return empty list"""
        result = load_data('nonexistent_file.json')
        assert isinstance(result, list)
        assert len(result) == 0


if __name__ == '__main__':
    pytest.main([__file__, '-v'])
