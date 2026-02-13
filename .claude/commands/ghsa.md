Analyze and help fix the GitHub Security Advisory (GHSA) at: $ARGUMENTS

Steps:
1. Fetch the GHSA page using `gh api repos/gogs/gogs/security-advisories` and understand the vulnerability details (description, severity, affected versions, CWE).
2. Verify the reported vulnerability actually exists, and why.
3. Identify the affected code in this repository.
4. Propose a fix with a clear explanation of the root cause and how the fix addresses it. Check for prior art in the codebase to stay consistent with existing patterns.
5. Implement the fix. Only add tests when there is something meaningful to test at our layer.
6. Run all the usual build and test commands.
7. Create a branch named after the GHSA ID, commit, and push.
8. Create a pull request with a proper title and description, do not reveal too much detail and link the GHSA.
