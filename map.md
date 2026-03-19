

## The fix: give the agent a persistent project map

The single most effective change is creating an `AGENT.md` file (sometimes called `CLAUDE.md`, `CONTEXT.md`, or `.agent`) at the root of your project. This is a **living document the agent reads first, updates last**.

```markdown
# Project map

## Stack
- Framework: Next.js 14, App Router
- Styling: Tailwind CSS + shadcn/ui
- Auth: NextAuth v5
- DB: Prisma + PostgreSQL

## Folder structure
src/
  app/          → routes (App Router convention)
  components/   → shared UI components
  lib/          → utilities, db client, auth config
  hooks/        → custom React hooks

## Naming conventions
- Components: PascalCase, one per file
- Hooks: useXxx.ts
- DB queries: lib/queries/xxx.ts

## Key files
- src/lib/auth.ts       → NextAuth config
- src/lib/db.ts         → Prisma client singleton
- src/app/layout.tsx    → root layout, providers

## Current work
- Feature: user dashboard (in progress, branch: feat/dashboard)
- Known issue: session refresh flickers on mobile

## Patterns in use
- Server components by default, "use client" only when needed
- All forms use react-hook-form + zod
- API routes: src/app/api/[route]/route.ts
```

The agent reads this at the start of every task. It now knows the terrain without scanning anything.

---

## The agent's discipline: targeted reads, not full scans

Once the project map exists, the agent's rule should be:

**Never `ls` the whole project. Ask the map, then read one file.**

The mental hierarchy:
1. "Does the map tell me where this lives?" → use the map
2. "I know the pattern, let me find the specific file" → `grep -r "keyword" src/` on one concept, not the whole tree
3. "I need to read this exact file" → read it and only it

If the agent finds itself reading more than 3–4 files before writing anything, it's exploring too broadly. That's a sign the project map needs to be richer.

---

## The agent's discipline: update the map after every task

This is the habit that makes the whole system compound. After completing a task, the agent should ask: "Did I learn anything that should be in the map?"

Examples of things worth capturing:
- A new pattern it introduced ("added Zod validation schema in `lib/schemas/`")
- A gotcha it hit ("Prisma client must be imported from `lib/db.ts`, not instantiated directly")
- A file that turned out to be load-bearing ("don't touch `middleware.ts` — it handles auth for all routes")
- A current state update ("login flow is now complete, starting on dashboard")

The map becomes more valuable with every task. An agent that uses it well becomes faster over time, not slower.

---

## The agent's inner monologue before every task

Train your agent to always answer these four questions before touching code:

1. **What does the map say about this area?** (don't ignore it)
2. **What is the single file most likely to be affected?** (start there)
3. **What existing pattern should I follow?** (match, don't invent)
4. **What is the smallest change that solves this?** (don't over-reach)

If the agent can't answer question 2 from the map alone, then — and only then — it does a targeted search. A `grep` for the component name, not a full directory walk.

---

## Practical prompt instruction for your agent

If you control the system prompt, add something like this:

```
Before starting any task:
1. Read AGENT.md to load the project context.
2. Identify the ONE most relevant file — do not scan the whole project.
3. Read only that file (and imports if needed).
4. Make the change.
5. After completing the task, update AGENT.md if you learned anything new.

Never run `ls -la` or `find .` at the start of a task unless AGENT.md does not exist yet.
```

That single instruction eliminates most of the redundant scanning.

---

## If you can't use a file: build a mental schema rule

If your agent doesn't have file system access or you can't add `AGENT.md`, the equivalent is a **schema in the system prompt** — a compact, structured description of the project that the agent always has in context. Same idea: stack, folders, patterns, key files. Just living in the prompt instead of on disk.

The principle is the same either way: **the agent should arrive at every task already knowing the shape of the project**, and only fetch the specific details it needs to complete that one task.