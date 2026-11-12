# Token comparison review notes

- Preserve the golden scripts when optimizing frontier storage, prefix or
  suffix handling, or backtracking. A different shortest script can change
  identities computed by callers even when it produces the same final text;
  deletion ties must agree in the forward pass and backtracking.
- Charge trace cells before allocation and return no partial script on
  exhaustion. A caller must be able to distinguish an unavailable comparison
  from a complete empty change.
