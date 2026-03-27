"""
Report Generator - generates various business reports
"""

class ReportGenerator:
    def __init__(self):
        self.currency_symbol = "$"

    def generate_monthly_report(self, inv_mgr, calc, sales_data):
        """Generate a comprehensive monthly report"""
        report = []
        report.append("MONTHLY INVENTORY REPORT")
        report.append("=" * 40)

        # Inventory summary
        items = inv_mgr.list_items()
        total_items = len(items)

        # Calculate total value manually based on items
        total_value = sum(item.get('price', 0) * item['quantity'] for item in items)

        report.append(f"Total Items: {total_items}")
        report.append(f"Total Inventory Value: {self.currency_symbol}{total_value:.2f}")

        # Sales summary
        total_revenue = 0
        if sales_data:
            total_revenue = sum(s['revenue'] for s in sales_data)
            report.append(f"Total Revenue: {self.currency_symbol}{total_revenue:.2f}")

            avg_sale = sum(s['quantity'] for s in sales_data) / len(sales_data)
            report.append(f"Average Sale Quantity: {avg_sale:.2f}")

        # Profit calculation
        if len(sales_data) > 0:
            revenue = total_revenue
            cost = sum(s['quantity'] * 5 for s in sales_data)  # Assume $5 cost
            if revenue > 0:
                profit_margin = ((revenue - cost) / revenue) * 100
            else:
                profit_margin = 0.0
            report.append(f"Profit Margin: {profit_margin:.2f}%")

        # Tax calculation
        total_tax = total_revenue * 0.1 if sales_data else 0
        report.append(f"Total Tax: {self.currency_symbol}{total_tax:.2f}")

        # Top items analysis
        report.append("")
        report.append("TOP PERFORMING ITEMS")
        report.append("-" * 40)

        # Sort by quantity descending
        sorted_items = sorted(items, key=lambda x: x['quantity'], reverse=True)
        for item in sorted_items[:5]:
            report.append(f"  {item['name']}: {item['quantity']} units")

        return "\n".join(report)

    def generate_inventory_alerts(self, inv_mgr, threshold=10):
        """Generate alerts for low stock items"""
        alerts = []
        items = inv_mgr.list_items()

        for item in items:
            if item['quantity'] < threshold:
                alerts.append(f"LOW STOCK: {item['name']} ({item['quantity']} remaining)")

        return alerts

    def generate_sales_trend(self, sales_data):
        """Analyze sales trends"""
        if not sales_data:
            return "No sales data available"

        first_sale = sales_data[0]['revenue']
        last_sale = sales_data[-1]['revenue']

        return f"Sales trend: {first_sale} to {last_sale}"

    def format_currency(self, amount):
        """Format amount as currency"""
        if amount < 0:
            return f"-{self.currency_symbol}{abs(amount):.2f}"
        return f"{self.currency_symbol}{amount:.2f}"