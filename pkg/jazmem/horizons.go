package jazmem

import (
	"fmt"
	"slices"

	"github.com/gluonfield/jazmem/internal/memfs"
)

// Memory horizons are root-level files injected into agent context every turn,
// not indexed pages. LONG_TERM.md is dream-maintained and read-only for
// agents; SHORT_TERM.md is agent-updated and dream-pruned.
const (
	LongTermFile      = memfs.LongTermFile
	ShortTermFile     = memfs.ShortTermFile
	LongTermMaxChars  = memfs.LongTermMaxChars
	ShortTermMaxChars = memfs.ShortTermMaxChars
)

func HorizonFiles() []string {
	return memfs.HorizonFiles()
}

func (m *Memory) ReadHorizonFile(name string) (string, error) {
	if !isHorizonFile(name) {
		return "", fmt.Errorf("unknown horizon file %q", name)
	}
	return m.fs.ReadRootFile(name)
}

func (m *Memory) WriteHorizonFile(name, content string) error {
	if !isHorizonFile(name) {
		return fmt.Errorf("unknown horizon file %q", name)
	}
	if err := memfs.ValidateHorizonContent(name, content); err != nil {
		return err
	}
	return m.fs.WriteRootFile(name, content)
}

func isHorizonFile(name string) bool {
	return slices.Contains(memfs.HorizonFiles(), name)
}
