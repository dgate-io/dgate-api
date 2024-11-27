package admin

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/dgate-io/dgate-api/internal/admin/changestate"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/hashicorp/raft"
	"go.uber.org/zap"
)

type AdminFSM struct {
	cs      changestate.ChangeState
	storage raft.StableStore
	logger  *zap.Logger

	localState *saveState
}

var _ raft.BatchingFSM = (*AdminFSM)(nil)

type saveState struct {
	AppliedIndex uint64 `json:"aindex"`
}

func newAdminFSM(
	logger *zap.Logger,
	storage raft.StableStore,
	cs changestate.ChangeState,
) raft.FSM {
	fsm := &AdminFSM{cs, storage, logger, &saveState{}}
	stateBytes, err := storage.Get([]byte("prev_state"))
	if err != nil {
		logger.Error("error getting prev_state", zap.Error(err))
	} else if len(stateBytes) != 0 {
		if err = json.Unmarshal(stateBytes, &fsm.localState); err != nil {
			logger.Warn("corrupted state detected", zap.ByteString("prev_state", stateBytes))
		} else {
			logger.Info("found state in store", zap.Any("prev_state", fsm.localState))
			return fsm
		}
	}
	return fsm
}

func (fsm *AdminFSM) applyLog(log *raft.Log, reload bool) (*spec.ChangeLog, error) {
	switch log.Type {
	case raft.LogCommand:
		var cl spec.ChangeLog
		if err := json.Unmarshal(log.Data, &cl); err != nil {
			fsm.logger.Error("Error unmarshalling change log", zap.Error(err))
			return nil, err
		}

		if cl.ID == "" {
			fsm.logger.Error("Change log ID is empty")
			return nil, errors.New("change log ID is empty")
		} else if cl.Cmd.IsNoop() {
			return nil, nil
		}
		// find a way to only reload if latest index to save time
		return &cl, fsm.cs.ProcessChangeLog(&cl, reload)
	case raft.LogConfiguration:
		servers := raft.DecodeConfiguration(log.Data).Servers
		fsm.logger.Debug("configuration update server",
			zap.Any("address", servers),
			zap.Uint64("index", log.Index),
			zap.Uint64("term", log.Term),
			zap.Time("appended", log.AppendedAt),
		)
	default:
		fsm.logger.Error("Unknown log type in FSM Apply")
	}
	return nil, nil
}

func (fsm *AdminFSM) Apply(log *raft.Log) any {
	if resps := fsm.ApplyBatch([]*raft.Log{log}); len(resps) == 1 {
		return resps[0]
	}
	panic("apply batch not returning the correct number of responses")
}

func (fsm *AdminFSM) ApplyBatch(logs []*raft.Log) []any {
	rft := fsm.cs.Raft()
	appliedIndex := rft.AppliedIndex()
	lastLogIndex := logs[len(logs)-1].Index
	fsm.logger.Debug("apply log batch",
		zap.Uint64("applied", appliedIndex),
		zap.Uint64("commit", rft.CommitIndex()),
		zap.Uint64("last", rft.LastIndex()),
		zap.Uint64("log[0]", logs[0].Index),
		zap.Uint64("log[-1]", lastLogIndex),
		zap.Int("logs", len(logs)),
	)

	var err error
	results := make([]any, len(logs))

	for i, log := range logs {
		isLast := len(logs)-1 == i
		reload := fsm.shouldReload(log, isLast)
		if _, err = fsm.applyLog(log, reload); err != nil {
			fsm.logger.Error("Error applying log", zap.Error(err))
			results[i] = err
		}
	}

	if appliedIndex != 0 && lastLogIndex >= appliedIndex {
		fsm.localState.AppliedIndex = lastLogIndex
		if err = fsm.saveFSMState(); err != nil {
			fsm.logger.Warn("failed to save applied index state",
				zap.Uint64("applied_index", lastLogIndex),
			)
		}
		// defer fsm.cs.SetReady(true)
	}

	return results
}

func (fsm *AdminFSM) saveFSMState() error {
	fsm.logger.Debug("saving localState",
		zap.Any("data", fsm.localState),
	)
	stateBytes, err := json.Marshal(fsm.localState)
	if err != nil {
		return err
	}
	return fsm.storage.Set([]byte("prev_state"), stateBytes)
}

func (fsm *AdminFSM) shouldReload(log *raft.Log, reload bool) bool {
	if reload {
		return log.Index >= fsm.localState.AppliedIndex
	}
	return false
}

func (fsm *AdminFSM) Snapshot() (raft.FSMSnapshot, error) {
	fsm.cs = nil
	fsm.logger.Warn("snapshots not supported")
	return nil, errors.New("snapshots not supported")
}

func (fsm *AdminFSM) Restore(rc io.ReadCloser) error {
	fsm.logger.Warn("snapshots not supported, cannot restore")
	return nil
}
