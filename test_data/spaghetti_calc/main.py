"""
Main entry point for the Inventory Management System
"""
import sys
from calculator import Calculator
from inventory import InventoryManager
from reports import ReportGenerator
from data_loader import load_data, save_data

def main():
    print("=" * 50)
    print("INVENTORY MANAGEMENT SYSTEM")
    print("=" * 50)

    # Load inventory data
    inventory_data = load_data("inventory.json")
    sales_data = load_data("sales.json")

    # Initialize components
    calc = Calculator()
    inv_mgr = InventoryManager(inventory_data)
    report_gen = ReportGenerator()

    while True:
        print("\n1. View Inventory")
        print("2. Add Item")
        print("3. Record Sale")
        print("4. Calculate Revenue")
        print("5. Generate Report")
        print("6. Exit")

        choice = input("\nSelect option: ")

        if choice == "1":
            items = inv_mgr.list_items()
            print("\nCurrent Inventory:")
            for item in items:
                print(f"  {item['name']}: {item['quantity']} @ ${item['price']}")

        elif choice == "2":
            name = input("Item name: ")
            qty = input("Quantity: ")
            price = input("Price: ")
            inv_mgr.add_item(name, int(qty), float(price))
            print(f"Added {name} to inventory")

        elif choice == "3":
            name = input("Item name: ")
            qty = input("Quantity sold: ")
            result = inv_mgr.record_sale(name, int(qty))
            if result['success']:
                print(f"Sale recorded: {result['revenue']}")
            else:
                print(f"Error: {result['error']}")

        elif choice == "4":
            total = calc.calculate_total_revenue(sales_data)
            print(f"Total Revenue: ${total}")

        elif choice == "5":
            report = report_gen.generate_monthly_report(inv_mgr, calc, sales_data)
            print("\n" + "=" * 50)
            print(report)
            print("=" * 50)

        elif choice == "6":
            save_data("inventory.json", inv_mgr.get_data())
            save_data("sales.json", sales_data)
            print("Data saved. Goodbye!")
            break

        else:
            print("Invalid option")

if __name__ == "__main__":
    main()
