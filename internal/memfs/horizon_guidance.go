package memfs

import "github.com/gluonfield/jazmem/internal/templates/memorypolicy"

func LongTermDreamGuidance() string {
	return memorypolicy.RenderLongTerm()
}

func ShortTermDreamGuidance() string {
	return memorypolicy.RenderShortTerm()
}
