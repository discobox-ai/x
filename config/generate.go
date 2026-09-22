package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// schemaDialect is the JSON Schema version Schema writes.
const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

// commentWidth is where Example wraps its comments.
const commentWidth = 78

// Generate writes the schema and example files for defaults, a Config struct
// value holding its defaults, into dir as <Name>.schema.json and
// <Name>.example.yaml. A program runs it from `go generate` and commits the
// files, so a stale one shows up as a diff.
func (s Spec) Generate(dir string, defaults any) error {
	if s.Name == "" {
		return errors.New("config: Spec.Name is required to generate files")
	}
	schema, err := s.Schema(defaults)
	if err != nil {
		return err
	}
	example, err := s.Example(defaults)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, s.Name+".schema.json"), schema, 0o644); err != nil { //nolint:gosec // a committed, world-readable doc
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.Name+".example.yaml"), example, 0o644) //nolint:gosec // a committed, world-readable doc
}

// Schema returns the JSON schema of the configuration file, with each key's
// documentation, default and environment variable.
func (s Spec) Schema(defaults any) ([]byte, error) {
	root, v, err := s.documented(defaults)
	if err != nil {
		return nil, err
	}
	schema, err := groupSchema(root, v)
	if err != nil {
		return nil, err
	}
	schema["$schema"] = schemaDialect
	if s.Name != "" {
		schema["title"] = s.Name + " configuration"
	}
	out, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Example returns a commented configuration file setting every key to its
// default. Loading it changes nothing.
func (s Spec) Example(defaults any) ([]byte, error) {
	root, v, err := s.documented(defaults)
	if err != nil {
		return nil, err
	}
	body, err := groupNode(root, v)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if s.Name != "" {
		fmt.Fprintf(&buf, "# yaml-language-server: $schema=%s.schema.json\n#\n", s.Name)
		fmt.Fprintf(&buf, "# %s configuration.\n#\n", s.Name)
	}
	buf.WriteString(comment("Every key is optional, and the value shown is its default. " +
		"The environment variable named beside a key overrides it. " +
		"Generated from the program's Config struct; do not edit."))
	buf.WriteString("\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	// yaml.v3 indents the blank line inside a nested mapping.
	lines := strings.Split(buf.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// documented describes defaults' type and requires every key to carry a doc
// tag, since the generated files are the configuration's documentation.
func (s Spec) documented(defaults any) (*setting, reflect.Value, error) {
	v := reflect.ValueOf(defaults)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, reflect.Value{}, fmt.Errorf("config: defaults must be a struct, not %T", defaults)
	}
	root, err := s.settings(v.Type())
	if err != nil {
		return nil, reflect.Value{}, err
	}
	var missing []string
	var walk func(*setting)
	walk = func(g *setting) {
		for _, c := range g.children {
			if strings.TrimSpace(c.doc) == "" {
				missing = append(missing, c.path)
			}
			walk(c)
		}
	}
	walk(root)
	if len(missing) > 0 {
		return nil, reflect.Value{}, fmt.Errorf("config: no doc tag on %s", strings.Join(missing, ", "))
	}
	return root, v, nil
}

// groupSchema is the object schema of a group of settings.
func groupSchema(group *setting, v reflect.Value) (map[string]any, error) {
	props := map[string]any{}
	for _, c := range group.children {
		var (
			prop map[string]any
			err  error
		)
		if c.kind == kindGroup {
			prop, err = groupSchema(c, v)
		} else {
			prop, err = leafSchema(c, v.FieldByIndex(c.index))
		}
		if err != nil {
			return nil, err
		}
		props[c.key] = prop
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if group.doc != "" {
		schema["description"] = group.doc
	}
	return schema, nil
}

// leafSchema is the schema of one setting, whose default is field.
func leafSchema(s *setting, field reflect.Value) (map[string]any, error) {
	prop := map[string]any{"description": leafDoc(s)}
	if s.kind == kindList {
		prop["type"] = "array"
		prop["items"] = map[string]any{"type": "string"}
		items, _ := reflect.TypeAssert[[]string](field)
		if items == nil {
			items = []string{}
		}
		prop["default"] = items
		return prop, nil
	}
	text, err := formatText(field)
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", s.path, err)
	}
	switch jsonType(s.typ) {
	case "boolean":
		prop["type"] = "boolean"
		prop["default"] = field.Bool()
	case "integer":
		prop["type"] = "integer"
		prop["default"] = json.Number(text)
	case "number":
		prop["type"] = "number"
		prop["default"] = json.Number(text)
	default:
		prop["type"] = "string"
		prop["default"] = text
	}
	return prop, nil
}

// groupNode is the YAML mapping of a group, each key headed by its
// documentation.
func groupNode(group *setting, v reflect.Value) (*yaml.Node, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}
	for i, c := range group.children {
		doc := c.doc
		if c.kind != kindGroup {
			doc = leafDoc(c)
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: c.key, HeadComment: strings.TrimSuffix(comment(doc), "\n")}
		if i > 0 {
			// A blank line between entries: yaml.v3 keeps a head comment's
			// leading blank line.
			key.HeadComment = "\n" + key.HeadComment
		}
		var (
			value *yaml.Node
			err   error
		)
		switch c.kind {
		case kindGroup:
			value, err = groupNode(c, v)
		case kindList:
			value = &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
			items, _ := reflect.TypeAssert[[]string](v.FieldByIndex(c.index))
			for _, item := range items {
				value.Content = append(value.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: item})
			}
		default:
			value, err = scalarNode(c, v.FieldByIndex(c.index))
		}
		if err != nil {
			return nil, err
		}
		m.Content = append(m.Content, key, value)
	}
	return m, nil
}

// scalarNode renders a default so that reading it back gives the same value.
func scalarNode(s *setting, field reflect.Value) (*yaml.Node, error) {
	text, err := formatText(field)
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", s.path, err)
	}
	tag := "!!str"
	switch jsonType(s.typ) {
	case "boolean":
		tag = "!!bool"
	case "integer":
		tag = "!!int"
	case "number":
		tag = "!!float"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: text}, nil
}

// leafDoc is a leaf's documentation with how the environment and a secret
// file set it.
func leafDoc(s *setting) string {
	doc := strings.TrimSpace(s.doc)
	if s.secret {
		doc += " Accepts file:<path> to read the value from a file."
	}
	return doc + " Environment: " + s.env + "."
}

// jsonType is the JSON schema type a scalar of type t is written as.
func jsonType(t reflect.Type) string {
	if t == durationType || reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return "string"
	}
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	}
	return "string"
}

// comment wraps text as "# " lines at commentWidth.
func comment(text string) string {
	var b strings.Builder
	line := "#"
	for word := range strings.FieldsSeq(text) {
		if len(line)+1+len(word) > commentWidth && line != "#" {
			b.WriteString(line + "\n")
			line = "#"
		}
		line += " " + word
	}
	b.WriteString(line + "\n")
	return b.String()
}
