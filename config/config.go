// Package config loads a server's configuration from one file plus
// environment overrides, and generates the JSON schema and commented example
// file that document it.
//
// A program declares its configuration as a Go struct, the one source of
// truth. Its keys are the fields' `json` names, and each field carries its
// documentation in a `doc` tag. The struct value a caller passes to Load holds
// the defaults. Load applies the file over them, then the environment over
// that, in one direction: whatever wins, always wins.
//
// Environment names are derived from key paths, as
// `<PREFIX>_<KEY_PATH>` in upper snake case, so `iroh.logLevel` under the
// BOUNCER prefix is BOUNCER_IROH_LOG_LEVEL. An `env` tag names the variable
// explicitly for the rare spelling an outside spec fixes, such as OTEL_*.
//
// Mistakes fail loudly: an unknown key in the file, and a variable carrying
// the prefix that matches no setting, are both errors naming what was
// wrong, so a misspelling is never silently the default.
//
// A string field tagged `secret:"true"` also accepts `file:<path>`, and the
// value is read from that file, which is how Kubernetes, systemd and Docker
// deliver secrets.
package config

import (
	"encoding"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Spec describes how one program's configuration is named and read.
type Spec struct {
	// Name is the program's name, such as "bouncer-server". It names the
	// generated files and heads them.
	Name string
	// Prefix is the environment prefix without its trailing underscore, such
	// as "BOUNCER".
	Prefix string
	// Ignore lists variables that carry Prefix but are not settings: a CLI's
	// variables when the CLI shares the prefix, or a test switch. Anything
	// else carrying the prefix must be a setting.
	Ignore []string
}

// secretFilePrefix marks a secret's value as the path of a file holding it.
const secretFilePrefix = "file:"

var (
	durationType        = reflect.TypeFor[time.Duration]()
	stringSliceType     = reflect.TypeFor[[]string]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// kind is the shape of a setting's value.
type kind int

const (
	kindScalar kind = iota // one value, written as text
	kindList               // a []string
	kindGroup              // a nested struct of settings
)

// setting is one key in the configuration, a leaf or a group of keys.
type setting struct {
	key      string // the key within its parent
	path     string // the dotted key path from the root
	index    []int  // the reflect field index from the root struct
	typ      reflect.Type
	kind     kind
	doc      string
	env      string     // the variable that overrides a leaf
	secret   bool       // a leaf string that accepts file:<path>
	children []*setting // a group's keys, in field order
}

// Load fills cfg, a pointer to the program's Config struct holding its
// defaults, from the file at path and then from environ.
//
// An empty path reads no file. environ is normally os.Environ(). Every
// problem found is reported, joined, rather than only the first.
func (s Spec) Load(path string, environ []string, cfg any) error {
	v := reflect.ValueOf(cfg)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config: Load needs a pointer to a struct, not %T", cfg)
	}
	root, err := s.settings(v.Elem().Type())
	if err != nil {
		return err
	}
	v = v.Elem()

	var errs []error
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		errs = append(errs, decodeFile(path, data, root, v)...)
	}
	errs = append(errs, s.applyEnv(environ, root, v)...)
	if len(errs) == 0 {
		errs = append(errs, readSecrets(root, v)...)
	}
	return errors.Join(errs...)
}

// applyEnv sets every leaf its variable names and rejects any variable
// carrying the prefix that names no setting.
func (s Spec) applyEnv(environ []string, root *setting, v reflect.Value) []error {
	byEnv := map[string]*setting{}
	for _, leaf := range leaves(root) {
		byEnv[leaf.env] = leaf
	}
	var errs []error
	for _, kv := range environ {
		name, value, _ := strings.Cut(kv, "=")
		leaf, ok := byEnv[name]
		if !ok {
			if strings.HasPrefix(name, s.Prefix+"_") && !slices.Contains(s.Ignore, name) {
				errs = append(errs, fmt.Errorf("environment variable %s matches no setting", name))
			}
			continue
		}
		field := v.FieldByIndex(leaf.index)
		if leaf.kind == kindList {
			field.Set(reflect.ValueOf(splitList(value)))
			continue
		}
		if err := setText(field, value); err != nil {
			errs = append(errs, fmt.Errorf("environment variable %s (%s): %w", name, leaf.path, err))
		}
	}
	return errs
}

// readSecrets replaces each secret holding file:<path> with the file's
// contents, less trailing line endings.
func readSecrets(root *setting, v reflect.Value) []error {
	var errs []error
	for _, leaf := range leaves(root) {
		if !leaf.secret {
			continue
		}
		field := v.FieldByIndex(leaf.index)
		path, ok := strings.CutPrefix(field.String(), secretFilePrefix)
		if !ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			// The error names the file, never the contents.
			errs = append(errs, fmt.Errorf("%s: read secret: %w", leaf.path, err))
			continue
		}
		field.SetString(strings.TrimRight(string(data), "\r\n"))
	}
	return errs
}

// settings describes the struct type t as the root group of settings.
func (s Spec) settings(t reflect.Type) (*setting, error) {
	if s.Prefix == "" {
		return nil, errors.New("config: Spec.Prefix is required")
	}
	root := &setting{typ: t, kind: kindGroup}
	if err := s.describe(root, nil); err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, leaf := range leaves(root) {
		if other, ok := seen[leaf.env]; ok {
			return nil, fmt.Errorf("config: %s and %s share the variable %s", other, leaf.path, leaf.env)
		}
		if slices.Contains(s.Ignore, leaf.env) {
			return nil, fmt.Errorf("config: %s's variable %s is in Spec.Ignore", leaf.path, leaf.env)
		}
		seen[leaf.env] = leaf.path
	}
	return root, nil
}

// describe fills group's children from its struct type's exported fields.
func (s Spec) describe(group *setting, keyPath []string) error {
	for i := range group.typ.NumField() {
		f := group.typ.Field(i)
		if !f.IsExported() {
			continue
		}
		key, skip := fieldKey(f)
		if skip {
			continue
		}
		if f.Anonymous {
			return fmt.Errorf("config: embedded field %s.%s is not supported; name it", group.typ, f.Name)
		}
		path := append(slices.Clone(keyPath), key)
		child := &setting{
			key:    key,
			path:   strings.Join(path, "."),
			index:  append(slices.Clone(group.index), i),
			typ:    f.Type,
			doc:    f.Tag.Get("doc"),
			secret: f.Tag.Get("secret") == "true",
		}
		switch {
		case isText(f.Type):
			child.kind = kindScalar
		case f.Type == stringSliceType:
			child.kind = kindList
		case f.Type.Kind() == reflect.Struct:
			child.kind = kindGroup
			if f.Tag.Get("env") != "" || child.secret {
				return fmt.Errorf("config: %s is a group; env and secret tags belong on its keys", child.path)
			}
			if err := s.describe(child, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("config: %s has unsupported type %s", child.path, f.Type)
		}
		if child.kind != kindGroup {
			child.env = f.Tag.Get("env")
			if child.env == "" {
				child.env = envName(s.Prefix, path)
			}
			if child.secret && f.Type.Kind() != reflect.String {
				return fmt.Errorf("config: secret %s must be a string", child.path)
			}
		}
		group.children = append(group.children, child)
	}
	return nil
}

// fieldKey is a field's key: its json name, or its Go name with the first
// letter lowered. skip reports a field tagged `json:"-"`.
func fieldKey(f reflect.StructField) (key string, skip bool) {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	switch name {
	case "-":
		return "", true
	case "":
		r := []rune(f.Name)
		r[0] = unicode.ToLower(r[0])
		return string(r), false
	}
	return name, false
}

// envName derives a leaf's variable from its key path: PREFIX_KEY_PATH.
func envName(prefix string, path []string) string {
	parts := []string{prefix}
	for _, key := range path {
		parts = append(parts, upperSnake(key))
	}
	return strings.Join(parts, "_")
}

// upperSnake turns a camelCase key into UPPER_SNAKE, keeping an acronym
// together: logLevel is LOG_LEVEL and databaseDSN is DATABASE_DSN.
func upperSnake(key string) string {
	r := []rune(key)
	var b strings.Builder
	for i, c := range r {
		if c == '-' || c == '_' {
			b.WriteByte('_')
			continue
		}
		if i > 0 && unicode.IsUpper(c) {
			prev := r[i-1]
			nextLower := i+1 < len(r) && unicode.IsLower(r[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(c))
	}
	return b.String()
}

// leaves lists a group's leaf settings, depth first in field order.
func leaves(group *setting) []*setting {
	var out []*setting
	for _, c := range group.children {
		if c.kind == kindGroup {
			out = append(out, leaves(c)...)
		} else {
			out = append(out, c)
		}
	}
	return out
}

// isText reports whether a value of type t is read from one piece of text.
func isText(t reflect.Type) bool {
	if t == durationType || reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// setText parses text into the scalar field v.
func setText(v reflect.Value, text string) error {
	if u, ok := reflect.TypeAssert[encoding.TextUnmarshaler](v.Addr()); ok {
		return u.UnmarshalText([]byte(text))
	}
	if v.Type() == durationType {
		d, err := time.ParseDuration(text)
		if err != nil {
			return err
		}
		v.SetInt(int64(d))
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(text)
	case reflect.Bool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return fmt.Errorf("%q is not a boolean", text)
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(text, 0, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a %s", text, v.Type())
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(text, 0, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a %s", text, v.Type())
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(text, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a number", text)
		}
		v.SetFloat(n)
	default:
		return fmt.Errorf("unsupported type %s", v.Type())
	}
	return nil
}

// splitList reads a list from a variable: comma-separated, each item trimmed.
// An empty value is an empty list.
func splitList(value string) []string {
	items := []string{}
	if strings.TrimSpace(value) == "" {
		return items
	}
	for item := range strings.SplitSeq(value, ",") {
		items = append(items, strings.TrimSpace(item))
	}
	return items
}

// formatText renders the scalar field v as the text setText reads back.
func formatText(v reflect.Value) (string, error) {
	if m, ok := reflect.TypeAssert[encoding.TextMarshaler](v); ok {
		b, err := m.MarshalText()
		return string(b), err
	}
	if v.CanAddr() {
		if m, ok := reflect.TypeAssert[encoding.TextMarshaler](v.Addr()); ok {
			b, err := m.MarshalText()
			return string(b), err
		}
	}
	if v.Type() == durationType {
		return time.Duration(v.Int()).String(), nil
	}
	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits()), nil
	}
	return "", fmt.Errorf("unsupported type %s", v.Type())
}
