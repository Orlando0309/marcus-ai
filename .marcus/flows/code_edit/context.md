Before editing:
- Read the file carefully and understand the surrounding structure.
- Match the nearest existing pattern instead of inventing a new one.
- Prefer the smallest diff that satisfies the instruction.
- If the request looks like debugging, reason from the observed failure before changing code.
- Keep comments focused on why, not what.

After editing:
- Sanity-check that the diff is coherent and verify with the narrowest useful build or test step.
