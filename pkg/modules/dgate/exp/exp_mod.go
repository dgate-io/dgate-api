package exp

import (
	"github.com/dgate-io/dgate-api/pkg/modules"
)

type ExperimentalModule struct {
	modCtx modules.RuntimeContext
}

var _ modules.GoModule = &ExperimentalModule{}

func New(modCtx modules.RuntimeContext) modules.GoModule {
	return &ExperimentalModule{modCtx}
}

func (hp *ExperimentalModule) Exports() *modules.Exports {
	return &modules.Exports{
		Named: map[string]any{},
	}
}
