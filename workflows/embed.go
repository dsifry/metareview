// Package workflows ships the default FSM workflow definitions embedded in the binary.
// Names resolve only to embedded files; paths are the caller's business.
package workflows

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed *.yaml
var files embed.FS

// Names lists the embedded workflow names (file names without the .yaml suffix), sorted.
func Names() []string {
	entries, _ := files.ReadDir(".") // the embedded tree always reads
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names
}

// Read returns the YAML bytes of an embedded workflow by name.
func Read(name string) ([]byte, error) {
	if strings.ContainsAny(name, "/\\.") {
		return nil, fmt.Errorf("workflows: %q is not an embedded workflow name", name)
	}
	b, err := files.ReadFile(name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("workflows: unknown workflow %q", name)
	}
	return b, nil
}
