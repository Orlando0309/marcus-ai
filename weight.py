#!/usr/bin/env python3
"""Simple weight conversion CLI."""


def kg_to_lbs(kg: float) -> float:
    return kg * 2.20462


def lbs_to_kg(lbs: float) -> float:
    return lbs / 2.20462


def g_to_oz(g: float) -> float:
    return g * 0.035274


def oz_to_g(oz: float) -> float:
    return oz / 0.035274


def kg_to_stone(kg: float) -> float:
    return kg * 0.157473


def stone_to_kg(stone: float) -> float:
    return stone / 0.157473


CONVERSIONS = {
    "1": ("kg to lbs", kg_to_lbs, "kg"),
    "2": ("lbs to kg", lbs_to_kg, "lbs"),
    "3": ("g to oz", g_to_oz, "g"),
    "4": ("oz to g", oz_to_g, "oz"),
    "5": ("kg to stone", kg_to_stone, "kg"),
    "6": ("stone to kg", stone_to_kg, "stone"),
}


def main():
    print("Weight Conversion CLI")
    print("-" * 30)
    for key, (name, _, unit) in CONVERSIONS.items():
        print(f"  {key}. {name}")
    print()

    choice = input("Select conversion (1-6): ").strip()
    if choice not in CONVERSIONS:
        print("Invalid choice.")
        return

    try:
        value = float(input("Enter value: ").strip())
    except ValueError:
        print("Invalid number.")
        return

    name, func, unit = CONVERSIONS[choice]
    result = func(value)
    print(f"\nResult: {value} {unit} = {result:.4f}")


if __name__ == "__main__":
    main()
