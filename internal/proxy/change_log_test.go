package proxy

import (
	"strconv"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/spec"
	"go.uber.org/zap"
)

func TestCompactChangeLog_DifferentNamespace(t *testing.T) {
	cmds := []spec.Command{
		spec.AddModuleCommand,
		spec.AddServiceCommand,
		spec.AddRouteCommand,
	}
	for _, cmd := range cmds {
		t.Run("test "+cmd.String(), func(tt *testing.T) {
			logs := []*spec.ChangeLog{
				{
					Cmd:       cmd,
					Name:      "test1",
					Namespace: "test-ns1",
				},
				{
					Cmd:       cmd,
					Name:      "test1",
					Namespace: "test-ns2",
				},
				{
					Cmd:       cmd,
					Name:      "test1",
					Namespace: "test-ns3",
				},
			}
			setSequentialChangeLogs(logs)
			removeList := compactChangeLogsRemoveList(zap.NewNop(), logs)
			testChangeLogRemoveList(tt, removeList)
		})
	}
}

func TestCompactChangeLog_SameNamespace(t *testing.T) {
	cmds := []spec.Command{
		spec.AddModuleCommand,
		spec.AddServiceCommand,
		spec.AddRouteCommand,
	}
	for _, cmd := range cmds {
		t.Run("test "+cmd.String(), func(tt *testing.T) {
			logs := []*spec.ChangeLog{
				{
					Cmd:       cmd,
					Name:      "test1",
					Namespace: "test-ns",
				},
				{
					Cmd:       cmd,
					Name:      "test1",
					Namespace: "test-ns",
				},
				{
					Cmd:       cmd,
					Name:      "test1",
					Namespace: "test-ns",
				},
			}
			setSequentialChangeLogs(logs)
			removeList := compactChangeLogsRemoveList(zap.NewNop(), logs)
			testChangeLogRemoveList(tt, removeList, 0, 1)
		})
	}
}

func TestCompactChangeLog_Mirror(t *testing.T) {
	logs := []*spec.ChangeLog{
		newCommonChangeLog(spec.AddNamespaceCommand),
		newCommonChangeLog(spec.AddDomainCommand),
		newCommonChangeLog(spec.AddServiceCommand),
		newCommonChangeLog(spec.AddRouteCommand),
		newCommonChangeLog(spec.AddModuleCommand),
		newCommonChangeLog(spec.DeleteModuleCommand),
		newCommonChangeLog(spec.DeleteRouteCommand),
		newCommonChangeLog(spec.DeleteServiceCommand),
		newCommonChangeLog(spec.DeleteDomainCommand),
		newCommonChangeLog(spec.DeleteNamespaceCommand),
	}
	setSequentialChangeLogs(logs)
	removeList := compactChangeLogsRemoveList(zap.NewNop(), logs)
	testChangeLogRemoveList(t, removeList, 4, 5, 3, 6, 2, 7, 1, 8, 0, 9)
}

func TestCompactChangeLog_Noop(t *testing.T) {
	logs := []*spec.ChangeLog{
		newCommonChangeLog(spec.NoopCommand),
		newCommonChangeLog(spec.NoopCommand),
		newCommonChangeLog(spec.NoopCommand),
		newCommonChangeLog(spec.AddDomainCommand),
		newCommonChangeLog(spec.AddServiceCommand),
		newCommonChangeLog(spec.NoopCommand),
		newCommonChangeLog(spec.AddRouteCommand),
		newCommonChangeLog(spec.AddModuleCommand),
	}
	setSequentialChangeLogs(logs)
	removeList := compactChangeLogsRemoveList(zap.NewNop(), logs)
	testChangeLogRemoveList(t, removeList, 0, 1, 2, 5)
}

func TestCompactChangeLog_AddDelete(t *testing.T) {
	logs := []*spec.ChangeLog{
		newCommonChangeLog(spec.AddNamespaceCommand),
		newCommonChangeLog(spec.DeleteNamespaceCommand),
	}
	setSequentialChangeLogs(logs)
	removeList := compactChangeLogsRemoveList(zap.NewNop(), logs)
	testChangeLogRemoveList(t, removeList, 0, 1)
}

// TODO: Add test cases for DiffNamespaces for all tests cases when that is implemented (like the one below)
func TestCompactChangeLog_AddDeleteDiffNamespaces(t *testing.T) {
	t.Skip()
	// logs := []*spec.ChangeLog{
	// 	newCommonChangeLog(spec.AddNamespaceCommand, "t1", "test-ns1"),
	// 	newCommonChangeLog(spec.AddNamespaceCommand, "t2", "test-ns2"),
	// 	newCommonChangeLog(spec.DeleteNamespaceCommand, "t1", "test-ns1"),
	// 	newCommonChangeLog(spec.DeleteNamespaceCommand, "t2", "test-ns2"),
	// }
	// setSequentialChangeLogs(logs)
	// removeList := compactChangeLogsRemoveList(zap.NewNop(), logs)
	// testChangeLogRemoveList(t, removeList, 0, 1, 2, 3)
}

func newCommonChangeLog(cmd spec.Command, others ...string) *spec.ChangeLog {
	cl := &spec.ChangeLog{
		Cmd:       cmd,
		Name:      "test1",
		Namespace: "test-ns",
	}
	if len(others) > 0 && others[0] != "" {
		cl.Name = others[0]
	}
	if len(others) > 1 && others[1] != "" {
		cl.Namespace = others[1]
	}
	return cl
}

func setSequentialChangeLogs(logs []*spec.ChangeLog) {
	for i, log := range logs {
		log.ID = strconv.FormatInt(int64(i), 36)
		log.Item = i
	}
}

func testChangeLogRemoveList(t *testing.T, logs []*spec.ChangeLog, order ...int) {
	logItems := make([]any, 0, len(logs))
	for _, log := range logs {
		logItems = append(logItems, log.Item)
	}
	t.Logf("logItems: %v", logItems)
	if len(logs) != len(order) {
		t.Fatalf("length mismatch, expected: %v but got: %v", len(order), len(logs))
		return
	}
	for i, logItem := range logItems {
		if logItem != order[i] {
			t.Errorf("order mismatch: expected '%v' next, but got '%v'", order, logItems)
			t.FailNow()
			return
		}
	}
}
