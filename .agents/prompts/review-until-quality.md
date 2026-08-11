# Review Until Quality

After implementation, review the result as if you are blocking a pull request.

Loop until the answer to each question is yes:

- Is the change specific to this repository?
- Are the instructions concrete and testable?
- Are duplicate or vague statements removed?
- Is the implementation simpler than the obvious more-abstract alternative?
- Do the tests prove the changed behavior?
- Do commands and documented expectations match the repository?
- Are validation results real rather than assumed?
- Would a future agent know what to read first and what to ignore?

If any answer is no, fix the files and review again.
