package compose

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// unread lists the keys of a mapping that aren't in read, leaving out x- extensions.
func unread(n *yaml.Node, read ...string) []string {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	var out []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i].Value; !slices.Contains(read, k) && !strings.HasPrefix(k, "x-") {
			out = append(out, k)
		}
	}
	return out
}

var deployWhy = map[string]string{
	"restart_policy": ", because Skiff restarts a service whenever it stops",
}

// ignoredSettings names each setting inside a service's keys that Skiff doesn't read, so none is dropped without a word. Keys Skiff doesn't read at all are reported by the plan.
func ignoredSettings(service string, node *yaml.Node) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, service+": "+fmt.Sprintf(format, a...)) }

	for _, k := range unread(mapValue(node, "build"), "context", "dockerfile", "args", "target") {
		add("build %s is ignored", k)
	}

	deploy := mapValue(node, "deploy")
	for _, k := range unread(deploy, "replicas", "resources") {
		add("deploy %s is ignored%s", k, deployWhy[k])
	}
	resources := mapValue(deploy, "resources")
	if slices.Contains(unread(resources, "limits"), "reservations") {
		add("deploy reservations are ignored, because Skiff sets limits, not reservations")
	}
	for _, k := range unread(mapValue(resources, "limits"), "memory", "cpus") {
		add("deploy limit %s is ignored", k)
	}

	if keys := unread(mapValue(node, "healthcheck"), "test", "disable"); len(keys) > 0 {
		add("healthcheck %s %s ignored, because Skiff runs its own check", strings.Join(keys, ", "), plural(keys, "is", "are"))
	}

	if deps := mapValue(node, "depends_on"); deps != nil && deps.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(deps.Content); i += 2 {
			dep, d := deps.Content[i].Value, deps.Content[i+1]
			if c := mapValue(d, "condition"); c != nil && c.Value == "service_completed_successfully" {
				add("waiting for %s to finish isn't supported yet, so %s runs as a long-lived service and this one only waits for it to start", dep, dep)
			}
			for _, k := range unread(d, "condition") {
				add("depends_on %s for %s is ignored", k, dep)
			}
		}
	}

	if ports := mapValue(node, "ports"); ports != nil && ports.Kind == yaml.SequenceNode {
		for _, p := range ports.Content {
			for _, k := range unread(p, "target", "published", "protocol", "host_ip") {
				add("port setting %s is ignored", k)
			}
		}
	}

	if vols := mapValue(node, "volumes"); vols != nil && vols.Kind == yaml.SequenceNode {
		for _, v := range vols.Content {
			if v.Kind == yaml.ScalarNode {
				if parts := strings.Split(v.Value, ":"); len(parts) > 2 && slices.Contains(strings.Split(parts[2], ","), "nocopy") {
					add("nocopy on %s is ignored, so a new volume starts with what the image has there", parts[1])
				}
				continue
			}
			target := ""
			if t := mapValue(v, "target"); t != nil {
				target = t.Value
			}
			for _, k := range unread(v, "type", "source", "target", "read_only", "tmpfs") {
				for _, sub := range subKeys(mapValue(v, k)) {
					add("volume setting %s.%s on %s is ignored", k, sub, target)
				}
				if mapValue(v, k).Kind != yaml.MappingNode {
					add("volume setting %s on %s is ignored", k, target)
				}
			}
			for _, k := range unread(mapValue(v, "tmpfs"), "size") {
				add("tmpfs setting %s on %s is ignored", k, target)
			}
		}
	}

	for _, kind := range []string{"secrets", "configs"} {
		if refs := mapValue(node, kind); refs != nil && refs.Kind == yaml.SequenceNode {
			for _, r := range refs.Content {
				if keys := unread(r, "source", "target"); len(keys) > 0 {
					add("%s %s %s ignored, so the file keeps the owner and mode it has in the repository", strings.TrimSuffix(kind, "s"), strings.Join(keys, ", "), plural(keys, "is", "are"))
				}
			}
		}
	}

	if nets := mapValue(node, "networks"); nets != nil && nets.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(nets.Content); i += 2 {
			if aliases := mapValue(nets.Content[i+1], "aliases"); aliases != nil && len(aliases.Content) > 0 {
				add("network aliases are ignored, so other services reach it as %s", service)
			}
		}
	}
	return out
}

// topLevelSettings checks the file's own keys besides services, secrets and configs.
func (f *File) topLevelSettings(root *yaml.Node) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, v := root.Content[i].Value, root.Content[i+1]
		switch {
		case key == "services" || key == "secrets" || key == "configs" || key == "version" || strings.HasPrefix(key, "x-"):
		case key == "include":
			f.Problems = append(f.Problems, "include isn't supported yet, so copy those services into this file")
		case key == "name":
			f.Notes = append(f.Notes, "the file's name is ignored, because the stack keeps the name you give it in Skiff")
		case key == "networks":
			if len(v.Content) > 0 {
				f.Notes = append(f.Notes, "networks are ignored: a stack's services share one private network and reach each other by service name")
			}
		case key == "volumes":
			for j := 0; j+1 < len(v.Content); j += 2 {
				name, def := v.Content[j].Value, v.Content[j+1]
				if ext := mapValue(def, "external"); ext != nil && ext.Value != "false" {
					f.Notes = append(f.Notes, fmt.Sprintf("volume %s is external, but Skiff creates a new empty one, so copy your data into it after the first deploy", name))
				}
				if keys := unread(def, "external"); len(keys) > 0 {
					f.Notes = append(f.Notes, fmt.Sprintf("volume %s: %s %s ignored, because Skiff keeps volumes on the server's disk", name, strings.Join(keys, ", "), plural(keys, "is", "are")))
				}
			}
		default:
			f.Notes = append(f.Notes, key+" is ignored")
		}
	}
}

func subKeys(n *yaml.Node) []string { return unread(n) }

func plural(items []string, one, many string) string {
	if len(items) == 1 {
		return one
	}
	return many
}
