#!/usr/bin/env python3
"""Universal conversion CLI - aggregates all conversion utilities."""

from weight import convert_weight, WEIGHT_UNITS

# TODO: Import height conversions
# TODO: Import volume conversions
# TODO: Import temperature conversions
# TODO: Import length conversions
# TODO: Import area conversions
# TODO: Import speed conversions
# TODO: Import time conversions
# TODO: Import data conversions (bytes, bits)

def main():
    print("=== Universal Converter ===")
    print("\nAvailable conversion types:")
    print("  1. Weight (kg, lbs, g, oz, stone)")
    print("  2. Height (coming soon)")
    print("  3. Volume (coming soon)")
    print("  4. Temperature (coming soon)")
    print("  5. Length (coming soon)")
    print("  6. Area (coming soon)")
    print("  7. Speed (coming soon)")
    print("  8. Time (coming soon)")
    print("  9. Data (coming soon)")
    
    choice = input("\nSelect conversion type (1-9): ").strip()
    
    if choice == "1":
        convert_weight()
    else:
        print("\nThis conversion type is not yet implemented.")
        print("Check back after completing the todo list!")

if __name__ == "__main__":
    main()
