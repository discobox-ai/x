package config

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// decodeFile applies a YAML (or JSON) config file over v. It walks the parsed
// document against the settings rather than unmarshaling into the struct, so
// an unknown key is reported with its full path and line, and every value is
// read by the same parser as its environment variable.
func decodeFile(path string, data []byte, root *setting, v reflect.Value) []error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []error{fmt.Errorf("%s: %w", path, err)}
	}
	if len(doc.Content) == 0 {
		return nil // an empty file sets nothing
	}
	return decodeGroup(path, resolve(doc.Content[0]), root, v)
}

// decodeGroup applies the mapping n to the group's settings.
func decodeGroup(file string, n *yaml.Node, group *setting, v reflect.Value) []error {
	if isNull(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return []error{fmt.Errorf("%s:%d: %s must be a mapping of keys", file, n.Line, describePath(group))}
	}
	var errs []error
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode, value := n.Content[i], resolve(n.Content[i+1])
		child := findChild(group, keyNode.Value)
		if child == nil {
			errs = append(errs, fmt.Errorf("%s:%d: unknown key %q", file, keyNode.Line, joinPath(group.path, keyNode.Value)))
			continue
		}
		if seen[child.key] {
			errs = append(errs, fmt.Errorf("%s:%d: duplicate key %q", file, keyNode.Line, child.path))
			continue
		}
		seen[child.key] = true
		errs = append(errs, decodeValue(file, value, child, v)...)
	}
	return errs
}

// decodeValue applies one key's value. A null value leaves the setting as it
// was.
func decodeValue(file string, n *yaml.Node, s *setting, v reflect.Value) []error {
	if s.kind == kindGroup {
		return decodeGroup(file, n, s, v)
	}
	if isNull(n) {
		return nil
	}
	field := v.FieldByIndex(s.index)
	if s.kind == kindList {
		if n.Kind != yaml.SequenceNode {
			return []error{fmt.Errorf("%s:%d: %s must be a list", file, n.Line, s.path)}
		}
		items := make([]string, 0, len(n.Content))
		for _, item := range n.Content {
			item = resolve(item)
			if item.Kind != yaml.ScalarNode {
				return []error{fmt.Errorf("%s:%d: %s must be a list of strings", file, item.Line, s.path)}
			}
			items = append(items, item.Value)
		}
		field.Set(reflect.ValueOf(items))
		return nil
	}
	if n.Kind != yaml.ScalarNode {
		return []error{fmt.Errorf("%s:%d: %s must be a single value", file, n.Line, s.path)}
	}
	if err := setText(field, n.Value); err != nil {
		return []error{fmt.Errorf("%s:%d: %s: %w", file, n.Line, s.path, err)}
	}
	return nil
}

// findChild returns the group's setting with the given key, or nil.
func findChild(group *setting, key string) *setting {
	for _, c := range group.children {
		if c.key == key {
			return c
		}
	}
	return nil
}

// resolve follows an alias to the node it names.
func resolve(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func describePath(s *setting) string {
	if s.path == "" {
		return "the file"
	}
	return s.path
}
