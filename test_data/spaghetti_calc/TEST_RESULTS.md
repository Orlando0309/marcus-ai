# Marcus Multi-File Debugging Test Results

## Test Date
2026-03-27

## Summary

**Result: SUCCESS** - Marcus successfully debugged a multi-file Python project with spaghetti code style.

---

## Test Setup

### Project Structure
```
spaghetti_calc/
├── main.py           # CLI entry point
├── calculator.py     # Mathematical operations (9 bugs)
├── inventory.py      # Inventory management (6 bugs)
├── reports.py        # Report generation (4 bugs)
├── data_loader.py    # File I/O (3 bugs)
├── inventory.json    # Sample data
├── sales.json        # Sample data
└── test_calculations.py  # 18 pytest tests
```

### Simple Prompt Used
> "The calculations are always wrong. Fix all calculation bugs."

---

## Bugs Found and Fixed

### calculator.py (9 bugs)
| Function | Bug | Fix |
|----------|-----|-----|
| `calculate_total_revenue` | `quantity - price` | `quantity * price` |
| `calculate_tax` | `amount / tax_rate` | `amount * tax_rate` |
| `calculate_discount` | `amount + discount` | `amount - discount` |
| `calculate_profit` | `(cost - revenue) / cost` | `(revenue - cost) / revenue` |
| `average` | `sum / (len + 1)` | `sum / len` |
| `percentage_change` | `/ new_value` | `/ old_value` |
| `weighted_average` | Missing division | Divide by sum of weights |
| `compound_interest` | Simple interest | `(1 + rate) ** periods` |
| `depreciation` | `value + depreciation` | `value - depreciation` |

### inventory.py (6 bugs)
| Function | Bug | Fix |
|----------|-----|-----|
| `remove_item` | `del inventory[i + 1]` | `del inventory[i]` |
| `record_sale` | `quantity < available` | `quantity > available` |
| `record_sale` | `quantity += sold` | `quantity -= sold` |
| `get_item` | Case sensitivity | Case-insensitive comparison |
| `get_total_value` | `qty + price` | `qty * price` |
| `update_price` | `price += new` | `price = new` |

### reports.py (4 bugs)
| Function | Bug | Fix |
|----------|-----|-----|
| `generate_inventory_alerts` | `quantity > threshold` | `quantity < threshold` |
| `generate_monthly_report` | Wrong value calc | Direct calculation |
| `generate_monthly_report` | Inverted sort | Descending sort |
| `format_currency` | No negative handling | Handle negatives |

### data_loader.py (3 bugs)
| Function | Bug | Fix |
|----------|-----|-----|
| `load_data` (missing file) | Return `{}` | Return `[]` |
| `load_data` (invalid JSON) | Return `None` | Return `[]` |
| `backup_data` | Write 100 chars | Write full content |

---

## Test Results

### All 18 Tests Passed

| Category | Tests | Status |
|----------|-------|--------|
| Calculator | 9 | PASS |
| Inventory Manager | 6 | PASS |
| Report Generator | 2 | PASS |
| Data Loader | 1 | PASS |
| **Total** | **18** | **100% PASS** |

---

## Key Changes Made to Marcus

### 1. Diff Parser Fix (`internal/diff/diff.go`)
```go
// Before: Strict validation that failed with LLM-generated line counts
if oldCount != p.OriginalLength {
    return fmt.Errorf("hunk @@ -%d,%d: old side has %d lines, expected %d", ...)
}

// After: Recalculate from actual hunk content
p.OriginalLength = oldCount
p.NewLength = newCount
return nil
```

### 2. Edit Command Improvements (`internal/cli/edit.go`)
- Changed prompt to request complete file content instead of diff format
- Added markdown code block stripping
- Increased max_tokens to 8192 for complete responses
- Generate diff internally from original to new content

---

## Scoring

| Metric | Score |
|--------|-------|
| Bugs Identified | 22/22 (100%) |
| Files Fixed | 4/4 (100%) |
| Tests Passing | 18/18 (100%) |
| Overall Rating | **Excellent** |

---

## Conclusion

Marcus demonstrated strong multi-file debugging capabilities:
- Successfully identified mathematical errors across multiple modules
- Fixed logic errors in conditionals and comparisons
- Corrected data integrity issues (file I/O, backups)
- Handled edge cases (empty lists, zero division, negative values)

The key insight was that asking Marcus to return **complete file content** rather than unified diffs produces more reliable results, as LLMs often generate approximate line counts in diff headers.
