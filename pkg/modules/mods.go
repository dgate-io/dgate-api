package modules

import (
	"context"

	"github.com/dgate-io/dgate-api/pkg/cache"
	"github.com/dgate-io/dgate-api/pkg/eventloop"
	"github.com/dgate-io/dgate-api/pkg/resources"
	"github.com/dgate-io/dgate-api/pkg/scheduler"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dop251/goja"
)

type Module interface {
	New(RuntimeContext) GoModule
}

type GoModule interface {
	Exports() *Exports
}

type Exports struct {
	// Default is what will be the `default` export of a module
	Default any
	// Named is the named exports of a module
	Named map[string]any
}

type StateManager interface {
	ApplyChangeLog(*spec.ChangeLog) error
	ResourceManager() *resources.ResourceManager
	DocumentManager() resources.DocumentManager
	Scheduler() scheduler.Scheduler
	SharedCache() cache.TCache
}

type RuntimeContext interface {
	Context() context.Context
	EventLoop() *eventloop.EventLoop
	Runtime() *goja.Runtime
	State() StateManager
}
