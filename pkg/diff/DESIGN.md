# Token comparison

- `Lines` compares exact string tokens. It does not split text, normalize
  whitespace, or discard terminators; the caller chooses the tokens.
- Myers frontiers advance in increasing edit distance. A snake greedily
  consumes equal tokens; tied frontier choices take deletion. Backtracking
  uses the same choice and returns equal/delete/insert operations in order.
- Every frontier is retained for backtracking. Before allocating it, charge
  its cell count against the caller's budget; also charge each diagonal
  visit and token comparison. Exhaustion returns `ErrLimit` and no script.
  The budget bounds operation counts and trace storage, not token byte size.
- Golden scripts fix the deterministic result. Exhaustive small sequences
  check both application and minimum edit count against a separate dynamic
  programming oracle. No Git process or presentation setting is involved.
