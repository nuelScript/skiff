package compose

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxFileDepth bounds include and extends chains across files.
const maxFileDepth = 8

// Reader reads a file named relative to the Compose file's folder. It is nil for a pasted file, which has no folder.
type Reader func(name string) ([]byte, error)

// load reads one Compose file into its root mapping, with aliases, merge keys and variables already applied.
func (f *File) load(data []byte, name string, lookup func(string) (string, bool)) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is empty", name)
	}
	expandAliases(&doc, map[*yaml.Node]bool{})
	stripMergeTags(&doc, map[*yaml.Node]bool{})
	if err := interpolate(&doc, lookup, f, map[*yaml.Node]bool{}); err != nil {
		return nil, err
	}
	return doc.Content[0], nil
}

// loadOther reads a file another one names, with its relative paths rewritten to start from the main file's folder, as Compose resolves them from the file that wrote them.
func (f *File) loadOther(name string, read Reader, lookup func(string) (string, bool)) (*yaml.Node, error) {
	data, err := read(name)
	if err != nil {
		return nil, fmt.Errorf("couldn't read %s", name)
	}
	root, err := f.load(data, name, lookup)
	if err != nil {
		return nil, err
	}
	rebaseFile(root, path.Dir(name))
	return root, nil
}

// applyIncludes adds the services, volumes, secrets and configs of each included file to root. Compose refuses a service defined twice, and so does this.
func (f *File) applyIncludes(root *yaml.Node, read Reader, lookup func(string) (string, bool), depth int) error {
	inc := removeKey(root, "include")
	if inc == nil {
		return nil
	}
	if read == nil {
		f.Problems = append(f.Problems, "include needs the files it names, which a pasted file doesn't have, so copy those services into this file")
		return nil
	}
	if depth >= maxFileDepth {
		return errors.New("include goes too deep, or in a circle")
	}
	if inc.Kind != yaml.SequenceNode {
		return errors.New("include should be a list of files")
	}
	for _, entry := range inc.Content {
		var names []string
		switch entry.Kind {
		case yaml.ScalarNode:
			names = []string{entry.Value}
		case yaml.MappingNode:
			p := mapValue(entry, "path")
			if p == nil {
				return errors.New("an include entry needs a path")
			}
			if p.Kind == yaml.SequenceNode {
				if len(p.Content) != 1 {
					f.Problems = append(f.Problems, "include with several paths, which override one another, isn't supported yet, so merge them into one file")
					continue
				}
				p = p.Content[0]
			}
			names = []string{p.Value}
			for _, k := range unread(entry, "path") {
				f.Notes = append(f.Notes, fmt.Sprintf("include %s for %s is ignored", k, strings.Join(names, ", ")))
			}
		}
		for _, name := range names {
			other, err := f.loadOther(name, read, lookup)
			if err != nil {
				return err
			}
			if err := f.applyIncludes(other, read, lookup, depth+1); err != nil {
				return err
			}
			if err := mergeTopLevel(root, other, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeTopLevel adds other's definitions to root. A service in both is an error; a volume, secret or config in both keeps root's.
func mergeTopLevel(root, other *yaml.Node, name string) error {
	for _, key := range []string{"services", "volumes", "secrets", "configs", "networks"} {
		add := mapValue(other, key)
		if add == nil || add.Kind != yaml.MappingNode {
			continue
		}
		into := mapValue(root, key)
		if into == nil {
			into = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, into)
		}
		for i := 0; i+1 < len(add.Content); i += 2 {
			k := add.Content[i].Value
			if mapValue(into, k) != nil {
				if key == "services" {
					return fmt.Errorf("service %s is in both %s and the file that includes it", k, name)
				}
				continue
			}
			into.Content = append(into.Content, add.Content[i], add.Content[i+1])
		}
	}
	return nil
}

func removeKey(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			v := n.Content[i+1]
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return v
		}
	}
	return nil
}

// rebaseFile rewrites the relative paths in a file read from dir: build contexts, env files, bind mounts, extends files, and secret and config files.
func rebaseFile(root *yaml.Node, dir string) {
	if dir == "." || dir == "" {
		return
	}
	if services := mapValue(root, "services"); services != nil && services.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(services.Content); i += 2 {
			rebaseService(services.Content[i+1], dir)
		}
	}
	for _, key := range []string{"secrets", "configs"} {
		if defs := mapValue(root, key); defs != nil && defs.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(defs.Content); i += 2 {
				rebaseScalar(mapValue(defs.Content[i+1], "file"), dir)
			}
		}
	}
	if inc := mapValue(root, "include"); inc != nil {
		for _, e := range inc.Content {
			if e.Kind == yaml.ScalarNode {
				rebaseScalar(e, dir)
				continue
			}
			p := mapValue(e, "path")
			if p != nil && p.Kind == yaml.SequenceNode {
				for _, n := range p.Content {
					rebaseScalar(n, dir)
				}
			} else {
				rebaseScalar(p, dir)
			}
		}
	}
}

func rebaseService(s *yaml.Node, dir string) {
	if s.Kind != yaml.MappingNode {
		return
	}
	if b := mapValue(s, "build"); b != nil {
		if b.Kind == yaml.ScalarNode {
			rebaseScalar(b, dir)
		} else if c := mapValue(b, "context"); c != nil {
			rebaseScalar(c, dir)
		} else {
			b.Content = append(b.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "context"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: dir})
		}
	}
	if ext := mapValue(s, "extends"); ext != nil {
		rebaseScalar(mapValue(ext, "file"), dir)
	}
	if env := mapValue(s, "env_file"); env != nil {
		switch env.Kind {
		case yaml.ScalarNode:
			rebaseScalar(env, dir)
		case yaml.SequenceNode:
			for _, e := range env.Content {
				if e.Kind == yaml.ScalarNode {
					rebaseScalar(e, dir)
				} else {
					rebaseScalar(mapValue(e, "path"), dir)
				}
			}
		}
	}
	if vols := mapValue(s, "volumes"); vols != nil && vols.Kind == yaml.SequenceNode {
		for _, v := range vols.Content {
			if v.Kind == yaml.MappingNode {
				if t := mapValue(v, "type"); t != nil && t.Value == "bind" {
					rebaseScalar(mapValue(v, "source"), dir)
				}
				continue
			}
			src, rest, ok := strings.Cut(v.Value, ":")
			if ok && (strings.HasPrefix(src, ".") || strings.Contains(src, "/")) && !strings.HasPrefix(src, "/") && !strings.HasPrefix(src, "~") && !strings.HasPrefix(src, "$") {
				v.Value = "./" + path.Join(dir, src) + ":" + rest
			}
		}
	}
}

// rebaseScalar puts dir in front of a relative path.
func rebaseScalar(n *yaml.Node, dir string) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Value == "" || path.IsAbs(n.Value) || strings.HasPrefix(n.Value, "~") || strings.Contains(n.Value, "://") {
		return
	}
	n.Value = path.Join(dir, n.Value)
}

// extendFromFile merges service base from another file into node.
func (f *File) extendFromFile(node *yaml.Node, name, file, base string, read Reader, lookup func(string) (string, bool), depth int) error {
	if read == nil {
		f.Problems = append(f.Problems, fmt.Sprintf("%s: extends %s from %s, which a pasted file can't read, so copy it into this file", name, base, file))
		removeKey(node, "extends")
		return nil
	}
	other, err := f.loadOther(file, read, lookup)
	if err != nil {
		return fmt.Errorf("service %s extends %s: %w", name, file, err)
	}
	services := mapValue(other, "services")
	if services == nil || mapValue(services, base) == nil {
		return fmt.Errorf("service %s extends %s, which isn't in %s", name, base, file)
	}
	if err := f.applyExtends(services, read, lookup, depth+1); err != nil {
		return err
	}
	*node = *mergeService(mapValue(services, base), node)
	return nil
}
