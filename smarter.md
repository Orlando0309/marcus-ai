

## The core shift: from "code writer" to "system thinker"

A naive agent writes code when asked. A good agent **models the system first** — it understands what already exists, where things belong, and what the consequences of a change are. Writing code is the last step, not the first.

---

## 1. Orientation — read before you write

Before touching anything, the agent should build a mental map:

- What language/framework is this? What version?
- What's the folder structure? Is there a `src/`, `lib/`, `tests/` separation?
- How does this project name things? (`camelCase`, `snake_case`, `kebab-case`?)
- Is there a linter, formatter, or CI config? (`.eslintrc`, `pyproject.toml`, `Makefile`)

This is "grep-driven archaeology" — reading `README`, checking `package.json` or `requirements.txt`, tracing the entry point to understand flow.

---

## 2. Finding the right file to modify

Good agents don't just create new files — they ask "does this already exist?" The method:

- Search for the concept, not just the filename. `grep -r "auth" src/` tells you more than looking for `auth.py`.
- Follow the import chain. If a function is called somewhere, find where it's defined.
- Check test files — they often document expected behavior better than comments do.
- When scaffolding a new feature, match the closest existing feature structurally. Don't invent a new pattern if one already exists.

---

## 3. Small, surgical changes

The golden rule: **the smaller the diff, the safer the change.** A 3-line diff is debuggable in 30 seconds. A 300-line diff is a liability.

This means:
- Separate refactoring from feature work. Do them in two steps.
- Don't "clean up while you're in there" unless that's the task.
- Prefer adding before replacing — add the new thing, verify it works, then remove the old.

---

## 4. Verification as a habit

Running code is not optional. Every change should be followed by:

- **Build check**: does it compile/parse?
- **Smoke test**: does the basic flow still work?
- **Targeted test**: does the specific thing I changed behave correctly?

Reading an error message *carefully* before responding to it is half the work. Most agents fix the wrong thing because they skim the stack trace.

---

## 5. Testing mindset

The key insight is: **tests describe intent, not implementation.** A good agent writes tests that say "given X, the system should produce Y" — not "this function should call this other function."

The agent should also think about *test placement*: is this a unit test (testing one function in isolation), an integration test (testing how two modules wire together), or an end-to-end test (testing a full user flow)? Each has a different cost and confidence tradeoff.

---

## 6. Debugging methodology

Good debugging is **hypothesis-driven**, not trial-and-error:

1. Reproduce the problem reliably first.
2. Form one hypothesis about the cause.
3. Add a single observation (a log, a print, a breakpoint) to confirm or deny it.
4. Only change code once the cause is confirmed.

The worst debugging pattern is changing multiple things at once. You can accidentally fix the bug and still not understand it — which means it comes back later.

---

## 7. Good practice as a mindset, not a checklist

Good code isn't about following rules — it's about **reducing the cognitive load for the next person** (who may be you in 6 months). This means:

- Variable and function names that explain their purpose, not their type.
- Comments that explain *why*, not *what* (the code shows what).
- Deleting unused code — dead code is misleading.
- Keeping functions short enough to be understood without scrolling.

