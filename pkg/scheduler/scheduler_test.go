package scheduler_test

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgate-io/dgate-api/pkg/scheduler"
	"github.com/stretchr/testify/assert"
)

func TestScheduleTask_Timeout(t *testing.T) {
	wg := sync.WaitGroup{}
	exeCount := atomic.Int32{}
	sch := scheduler.New(scheduler.Options{
		Interval: time.Millisecond * 20,
		AutoRun:  true,
	})

	for i := 0; i < 10; i++ {
		wg.Add(1)
		err := sch.ScheduleTask(strconv.Itoa(i), scheduler.TaskOptions{
			Timeout: time.Millisecond * 200,
			TaskFunc: func(_ context.Context) {
				exeCount.Add(1)
				wg.Done()
			},
		})
		if err != nil {
			t.Fail()
		}
	}

	assert.Equal(t, 0, int(exeCount.Load()))
	assert.Equal(t, 10, sch.TotalTasks())
	wg.Wait()
	assert.Equal(t, 10, int(exeCount.Load()))
	assert.Equal(t, 0, sch.TotalTasks())
}

func TestScheduleTask_Interval(t *testing.T) {
	exeCount := atomic.Int32{}
	sch := scheduler.New(scheduler.Options{
		Interval: time.Millisecond * 5,
		AutoRun:  true,
	})
	for i := 0; i < 10; i++ {
		err := sch.ScheduleTask(strconv.Itoa(i), scheduler.TaskOptions{
			Interval: time.Millisecond * 100,
			TaskFunc: func(_ context.Context) { exeCount.Add(1) },
		})
		if err != nil {
			t.Fail()
		}
		assert.Equal(t, i+1, sch.TotalTasks())
	}
	assert.Equal(t, 10, sch.TotalTasks())

	time.Sleep(time.Millisecond * 105)
	assert.Equal(t, 10, int(exeCount.Load()))

	time.Sleep(time.Millisecond * 105)
	assert.Equal(t, 20, int(exeCount.Load()))

	time.Sleep(time.Millisecond * 105)
	assert.Equal(t, 30, int(exeCount.Load()))

	for i := 0; i < 10; i++ {
		assert.Equal(t, 10-i, sch.TotalTasks())
		err := sch.StopTask(strconv.Itoa(i))
		if err != nil {
			t.Fail()
		}
	}

	time.Sleep(time.Millisecond * 105)
	assert.Equal(t, 0, sch.TotalTasks())
	assert.Equal(t, 30, int(exeCount.Load()))
}

func TestScheduleTask_Overwrite(t *testing.T) {
	sch := scheduler.New(scheduler.Options{
		Interval: time.Millisecond * 50,
		AutoRun:  true,
	})
	test1Flag := atomic.Bool{}
	test2Flag := atomic.Bool{}
	// ensure that test1Flag is not set
	err := sch.ScheduleTask("task1", scheduler.TaskOptions{
		Timeout: time.Millisecond * 50,
		TaskFunc: func(_ context.Context) {
			test1Flag.Store(true)
		},
	})
	assert.Nil(t, err)

	// ensure that ScheduleTask fails unless Overwrite is set
	err = sch.ScheduleTask("task1", scheduler.TaskOptions{})
	assert.ErrorIs(t, err, scheduler.ErrTaskAlreadyExists)

	// ensure that test2Flag is set
	err = sch.ScheduleTask("task1", scheduler.TaskOptions{
		Timeout: time.Millisecond * 50,
		TaskFunc: func(_ context.Context) {
			test2Flag.Store(true)
		},
		Overwrite: true,
	})
	assert.Nil(t, err)
	time.Sleep(time.Millisecond * 100)
	assert.False(t, test1Flag.Load())
	assert.True(t, test2Flag.Load())
}

func TestScheduleTask_TimeoutIntervalError(t *testing.T) {
	sch := scheduler.New(scheduler.Options{
		Interval: time.Millisecond * 10,
		AutoRun:  true,
	})
	err := sch.ScheduleTask("task1", scheduler.TaskOptions{
		Timeout:  time.Millisecond * 10,
		Interval: time.Millisecond * 10,
	})
	assert.ErrorIs(t, err, scheduler.ErrIntervalTimeoutBothSet)

	err = sch.ScheduleTask("task1", scheduler.TaskOptions{})
	assert.ErrorIs(t, err, scheduler.ErrIntervalTimeoutNoneSet)

	err = sch.ScheduleTask("task1", scheduler.TaskOptions{
		Timeout: time.Millisecond * 10,
	})
	assert.ErrorIs(t, err, scheduler.ErrTaskFuncNotSet)
}

func TestStopTask(t *testing.T) {
	sch := scheduler.New(scheduler.Options{})

	err := sch.ScheduleTask("task1", scheduler.TaskOptions{})
	assert.ErrorIs(t, err, scheduler.ErrSchedulerNotRunning)

	assert.Nil(t, sch.Start())

	err = sch.ScheduleTask("task1", scheduler.TaskOptions{
		Timeout: time.Second, TaskFunc: func(_ context.Context) {},
	})
	assert.Nil(t, err)

	err = sch.StopTask("task1")
	assert.Nil(t, err)

	err = sch.StopTask("task1")
	assert.ErrorIs(t, err, scheduler.ErrTaskNotFound)
}
