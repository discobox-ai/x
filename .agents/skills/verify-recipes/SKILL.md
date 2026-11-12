---
name: verify-recipes
description: Verify x's exported Go packages from a separate consumer module, including exact-token diff scripts and resource-limit errors.
---

# Public package verification

Exercise library changes from a temporary consumer module outside this tree.
Import the public package path and replace the module with this checkout only
in the temporary consumer's `go.mod`:

```sh
x_repo=$(pwd)
x_verify=$(mktemp -d)
cd "$x_verify"
go mod init example.org/x-verify
go mod edit -require=github.com/discobox-ai/x@v0.0.0
go mod edit -replace=github.com/discobox-ai/x="$x_repo"
```

Write a small `main.go` using the affected public exports, then `go run .`
and retain its output outside the product tree. Dependencies needed by the
consumer can be resolved with `go mod tidy` in that temporary directory.

For `github.com/discobox-ai/x/pkg/diff`, call `Lines` with repeated fences,
mixed CRLF/LF tokens and an absent final newline. Apply the returned operations
to reconstruct both sides and observe that their exact bytes remain intact.
Probe a budget of 1: `errors.Is(err, diff.ErrLimit)` must hold and the script
must be nil. Empty inputs with budget 0 return a complete empty script.
For `a,b` to `b,a`, observe deletion of `a`, equality of `b`, then insertion
of `a`; the deterministic tie choice matters to identity consumers.
