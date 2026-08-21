package id

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// Prefixes must not collide. An ID's type is meant to be recognizable on sight,
// and two resource types minting "sbx_" would make that quietly untrue.
//
// This is a test rather than a Register(prefix, description) registry on
// purpose. A registry catches the same mistake, but at init time, and buys it
// with global mutable state, init ordering, and constants that stop being
// constants. The mistake it guards against is a developer typing a prefix that
// already exists — which is caught here before the code runs, and without
// adding an accessor to the package just so a test can see its own constants.
//
// The const block is read from source so a new prefix is covered by existing
// here, rather than by someone remembering to add it to a list.
func TestPrefixesAreDistinct(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "id.go", nil, 0)
	if err != nil {
		t.Fatalf("parse id.go: %v", err)
	}
	seen := map[string]string{}
	found := 0
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			name := value.Names[0].Name
			if !strings.HasPrefix(name, "Prefix") {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			prefix, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			found++
			if previous, ok := seen[prefix]; ok {
				t.Errorf("%s and %s are both %q", previous, name, prefix)
			}
			seen[prefix] = name
		}
	}
	if found == 0 {
		t.Fatal("no Prefix constants found; has the const block moved?")
	}
	t.Logf("checked %d prefixes", found)
}
