package changestate

import (
	"github.com/dgate-io/dgate-api/internal/proxy"
	"github.com/dgate-io/dgate-api/pkg/raftadmin"
	"github.com/dgate-io/dgate-api/pkg/resources"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/hashicorp/raft"
)

type ChangeState interface {
	// Change state
	ApplyChangeLog(cl *spec.ChangeLog) error
	ProcessChangeLog(cl *spec.ChangeLog, reload bool) error
	WaitForChanges(cl *spec.ChangeLog) error
	ReloadState(bool, ...*spec.ChangeLog) error
	ChangeHash() uint64
	ChangeLogs() []*spec.ChangeLog

	// Readiness
	Ready() bool
	SetReady(bool)

	// Replication
	SetupRaft(*raft.Raft, *raftadmin.Client)
	Raft() *raft.Raft

	// Resources
	ResourceManager() *resources.ResourceManager
	DocumentManager() resources.DocumentManager
}

var _ ChangeState = (*proxy.ProxyState)(nil)
