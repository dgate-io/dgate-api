package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dgate-io/dgate-api/pkg/util/heap"
	"go.uber.org/zap"
)

type (
	TaskFunc      func(context.Context)
	priorityQueue = *heap.Heap[int64, *TaskDefinition]
)

type TaskOptions struct {
	// Interval is the time between each task run.
	// Timeout OR Interval must be set.
	Interval time.Duration
	// Timeout is the maximum time the task is allowed to run
	// before it is forcefully stopped.
	// Timeout OR Interval must be set.
	Timeout time.Duration
	// Overwrite indicates whether to overwrite the task if it already exists.
	// If set to false, an error will be returned if the task already exists.
	// If set to true, the task will be overwritten with the new task and any existing timers will be reset.
	Overwrite bool
	// TaskFunc is the function to run when the task is scheduled.
	TaskFunc TaskFunc
}

type Scheduler interface {
	Start() error
	Stop()
	Running() bool
	GetTask(string) (TaskDefinition, bool)
	ScheduleTask(string, TaskOptions) error
	StopTask(string) error
	TotalTasks() int
}

type scheduler struct {
	opts        Options
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *zap.Logger
	tasks       map[string]*TaskDefinition
	pendingJobs priorityQueue
	mutex       *sync.RWMutex
	running     bool
}

type TaskDefinition struct {
	Name     string
	Func     TaskFunc
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

var (
	ErrTaskAlreadyExists        = errors.New("task already exists")
	ErrTaskNotFound             = errors.New("task not found")
	ErrIntervalTimeoutBothSet   = errors.New("only one of Interval or Timeout must be set")
	ErrIntervalTimeoutNoneSet   = errors.New("either Interval or Timeout must be set")
	ErrTaskFuncNotSet           = errors.New("TaskFunc must be set")
	ErrIntervalDurationTooShort = errors.New("interval duration must be greater than 1 second")
	ErrTimeoutDurationTooShort  = errors.New("timeout duration must be greater than 1 second")
	ErrSchedulerRunning         = errors.New("scheduler is already running")
	ErrSchedulerNotRunning      = errors.New("scheduler is not running")
)

type Options struct {
	Interval time.Duration
	Logger   *zap.Logger
	AutoRun  bool
}

func New(opts Options) Scheduler {
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	return &scheduler{
		opts:        opts,
		ctx:         context.TODO(),
		logger:      opts.Logger,
		mutex:       &sync.RWMutex{},
		pendingJobs: heap.NewHeap[int64, *TaskDefinition](heap.MinHeapType),
		tasks:       make(map[string]*TaskDefinition),
	}
}

func (s *scheduler) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.running {
		return ErrSchedulerRunning
	}
	s.start()
	return nil
}

func (s *scheduler) start() {
	s.running = true
	s.ctx, s.cancel = context.WithCancel(s.ctx)
	go func() {
		ticker := time.NewTicker(s.opts.Interval)
		defer ticker.Stop()
		for range ticker.C {
			if func() (done bool) {
				s.mutex.Lock()
				defer s.mutex.Unlock()
			START:
				now := time.Now()
				taskDefTime, taskDef, ok := s.pendingJobs.Peak()
				if !ok {
					return
				}
				select {
				case <-s.ctx.Done():
					s.running = false
					done = true
					return
				case <-taskDef.ctx.Done():
					delete(s.tasks, taskDef.Name)
					s.pendingJobs.Pop()
					goto START
				default:
					tdt := time.UnixMicro(taskDefTime)
					if !tdt.After(now) {
						// Run the task
						s.pendingJobs.Pop()
						s.executeTask(tdt, taskDef)
						// Go to the start of the loop to check if there are any more tasks
						goto START
					}
				}
				return
			}() {
				break
			}
		}
	}()
}

func (s *scheduler) Running() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.running
}

func (s *scheduler) executeTask(tdt time.Time, taskDef *TaskDefinition) {
	defer func() {
		if taskDef.interval <= 0 {
			taskDef.cancel()
			delete(s.tasks, taskDef.Name)
		} else {
			µs := tdt.Add(taskDef.interval).UnixMicro()
			s.pendingJobs.Push(µs, taskDef)
		}
		if r := recover(); r != nil {
			s.logger.Error("panic occurred while executing task",
				zap.String("task_name", taskDef.Name), zap.Any("error", r))
		}
	}()
	taskDef.Func(taskDef.ctx)
}

func (s *scheduler) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.running {
		return
	}
	s.cancel()
}

func (s *scheduler) GetTask(taskId string) (TaskDefinition, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	td, ok := s.tasks[taskId]
	return *td, ok
}

func (s *scheduler) ScheduleTask(name string, opts TaskOptions) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		if !s.opts.AutoRun {
			return ErrSchedulerNotRunning
		}
		s.start()
	}

	if _, ok := s.tasks[name]; ok {
		if !opts.Overwrite {
			return ErrTaskAlreadyExists
		} else {
			s.stopTask(name)
		}
	}

	if opts.Interval == 0 && opts.Timeout == 0 {
		return ErrIntervalTimeoutNoneSet
	} else if opts.Interval != 0 && opts.Timeout != 0 {
		return ErrIntervalTimeoutBothSet
	} else if opts.Interval > 0 && opts.Interval < s.opts.Interval {
		return ErrIntervalDurationTooShort
	} else if opts.Timeout > 0 && opts.Timeout < s.opts.Interval {
		return ErrTimeoutDurationTooShort
	}

	if opts.TaskFunc == nil {
		return ErrTaskFuncNotSet
	}

	ctx, cancel := context.WithCancel(context.TODO())
	s.tasks[name] = &TaskDefinition{
		Name:     name,
		Func:     opts.TaskFunc,
		ctx:      ctx,
		cancel:   cancel,
		interval: opts.Interval,
	}
	µs := time.Now().Add(opts.Interval).UnixMicro()
	if opts.Timeout > 0 {
		µs = time.Now().Add(opts.Timeout).UnixMicro()
	}
	s.pendingJobs.Push(µs, s.tasks[name])
	return nil
}

func (s *scheduler) StopTask(name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.stopTask(name)
}

func (s *scheduler) stopTask(name string) error {
	if taskDef, ok := s.tasks[name]; !ok {
		return ErrTaskNotFound
	} else {
		taskDef.cancel()
	}
	delete(s.tasks, name)
	return nil
}

func (s *scheduler) TotalTasks() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.tasks)
}
