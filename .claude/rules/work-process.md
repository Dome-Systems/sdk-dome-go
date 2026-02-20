---
description: Work process rules — quality over speed, diagnose before fixing, issue+PR workflow
---

# Work Process

**Philosophy:** Quality over speed. Understand before changing. Deliver via PRs, not direct edits.

## 1. Diagnose Before Fixing

Before modifying any code:
- Read the relevant source files — don't guess from file names or memory
- Trace the actual execution path or data flow
- Identify the root cause, not just the symptom
- If the problem is unclear after investigation, say so — don't start editing speculatively

**Anti-pattern:** Changing code to "see if this fixes it," then changing it again when it doesn't. This creates noise, can introduce new bugs, and wastes time.

## 2. Plan Non-Trivial Changes

Use plan mode when:
- The change touches more than 2-3 files
- There's a design decision to make
- The scope or approach isn't obvious
- You're unfamiliar with the area of code

Skip planning only for:
- Single-line or obvious fixes (typos, clear bugs)
- Changes where the user gave exact instructions
- Pure research or investigation

## 3. No Hack-and-Retry Loops

If an approach fails:
- **First failure:** Analyze why it failed. Read error messages carefully. Check assumptions.
- **Second failure:** Step back. Re-examine the approach. Consider whether the mental model is wrong.
- **Still stuck:** Stop and communicate. Explain what you've tried, what you've learned, and ask for guidance.

Never:
- Make the same change with minor variations hoping one works
- Suppress errors or warnings to make something "pass"
- Work around a problem without understanding it

## 4. Separate Discovery from Fixing

When you discover an unrelated issue while working on something:
- **Do not fix it silently** in the current change
- Note it and create a GitHub issue with context
- Continue with the original task
- Exception: trivial adjacent fixes (obvious typo on the same line) can be included if noted in the commit

This keeps PRs focused and changes reviewable.

## 5. Deliver via Branch and PR

For all code changes:
- Create a branch following the naming convention (`marc/<issue-id>-<description>`)
- Make focused commits with clear messages
- Open a PR with a description that explains **why**, not just **what**
- Link to the relevant issue

Never:
- Commit directly to `main`
- Bundle unrelated changes in one PR
- Push to remote without explicit user approval

## 6. Commit and Push Discipline

- **Commits:** Create freely on branches as you work. Use conventional commit messages with scope.
- **Push:** Always confirm with the user before pushing to remote. "Ready to push" is a checkpoint, not an automatic action.
- **Force push:** Never, unless the user explicitly requests it.

## 7. Test Before Declaring Done

Before marking work complete:
- Run the relevant test suite (`make test`, `go test ./...`, etc.)
- If you changed behavior, verify the change works as intended
- If tests fail, fix them — don't mark the task complete with broken tests
- If you can't run tests (missing dependencies, environment issues), say so explicitly
