# Spaghetti Code Calculator Test

## Overview

This is a multi-file Python project with intentional bugs spread across multiple files. The code is written in a "spaghetti" style with interdependencies between modules.

## Files

| File | Description |
|------|-------------|
| `main.py` | Main entry point, CLI interface |
| `calculator.py` | Mathematical operations (revenue, tax, profit, etc.) |
| `inventory.py` | Inventory management |
| `reports.py` | Report generation |
| `data_loader.py` | JSON file loading/saving |
| `inventory.json` | Sample inventory data |
| `sales.json` | Sample sales data |

## Running the Application

```bash
cd test_data/spaghetti_calc
python main.py
```

## Test Scenarios

### Test 1: Revenue Calculation
```
Option 4: Calculate Revenue
Expected: Should show sum of (quantity * price) for all sales
Bug: Uses subtraction instead of multiplication
```

### Test 2: Record a Sale
```
Option 3: Record Sale
Expected: Should decrease inventory and calculate revenue
Bug: Increases inventory, wrong comparison logic
```

### Test 3: Generate Report
```
Option 5: Generate Report
Expected: Should show correct totals and profit margin
Bug: Multiple calculation errors cascade through report
```

## Bug Categories

1. **Mathematical Errors** - Wrong operators, formulas
2. **Logic Errors** - Inverted comparisons, wrong conditions
3. **Data Flow Errors** - Wrong data types returned
4. **Off-by-One Errors** - Index errors, count errors
5. **Edge Case Errors** - Missing null checks, empty handling

## Instructions for Marcus

Just tell Marcus something simple like:

> "The calculations in this inventory system are always wrong. Revenue is incorrect, profit margins are negative when they should be positive, and inventory quantities increase after sales instead of decreasing. Find and fix all the bugs."
