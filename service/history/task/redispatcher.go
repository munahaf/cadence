// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package task

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/backoff"
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/dynamicconfig"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/metrics"
)

const (
	defaultBufferSize = 200

	redispatchBackoffCoefficient  = 1.05
	redispatchMaxBackoffInternval = 2 * time.Minute
)

type (
	redispatchNotification struct {
		targetSize int
		doneCh     chan struct{}
	}

	// RedispatcherOptions configs redispatch interval
	RedispatcherOptions struct {
		TaskRedispatchInterval                  dynamicconfig.DurationPropertyFn
		TaskRedispatchIntervalJitterCoefficient dynamicconfig.FloatPropertyFn
	}

	redispatcherImpl struct {
		sync.Mutex

		taskProcessor Processor
		timeSource    clock.TimeSource
		options       *RedispatcherOptions
		logger        log.Logger
		metricsScope  metrics.Scope

		status          int32
		shutdownCh      chan struct{}
		shutdownWG      sync.WaitGroup
		redispatchCh    chan redispatchNotification
		redispatchTimer *time.Timer
		backoffPolicy   backoff.RetryPolicy
		taskQueues      map[int][]redispatchTask // priority -> redispatch queue
		taskChFull      map[int]bool             // priority -> if taskCh is full
	}

	redispatchTask struct {
		task           Task
		redispatchTime time.Time
	}
)

// NewRedispatcher creates a new task Redispatcher
func NewRedispatcher(
	taskProcessor Processor,
	timeSource clock.TimeSource,
	options *RedispatcherOptions,
	logger log.Logger,
	metricsScope metrics.Scope,
) Redispatcher {
	backoffPolicy := backoff.NewExponentialRetryPolicy(options.TaskRedispatchInterval())
	backoffPolicy.SetBackoffCoefficient(redispatchBackoffCoefficient)
	backoffPolicy.SetMaximumInterval(redispatchMaxBackoffInternval)
	backoffPolicy.SetExpirationInterval(backoff.NoInterval)

	return &redispatcherImpl{
		taskProcessor: taskProcessor,
		timeSource:    timeSource,
		options:       options,
		logger:        logger,
		metricsScope:  metricsScope,
		status:        common.DaemonStatusInitialized,
		shutdownCh:    make(chan struct{}),
		redispatchCh:  make(chan redispatchNotification, 1),
		backoffPolicy: backoffPolicy,
		taskQueues:    make(map[int][]redispatchTask),
		taskChFull:    make(map[int]bool),
	}
}

func (r *redispatcherImpl) Start() {
	if !atomic.CompareAndSwapInt32(&r.status, common.DaemonStatusInitialized, common.DaemonStatusStarted) {
		return
	}

	r.shutdownWG.Add(1)
	go r.redispatchLoop()

	r.logger.Info("Task redispatcher started.", tag.LifeCycleStarted)
}

func (r *redispatcherImpl) Stop() {
	if !atomic.CompareAndSwapInt32(&r.status, common.DaemonStatusStarted, common.DaemonStatusStopped) {
		return
	}

	close(r.shutdownCh)

	r.Lock()
	if r.redispatchTimer != nil {
		r.redispatchTimer.Stop()
	}
	r.redispatchTimer = nil
	r.Unlock()

	if success := common.AwaitWaitGroup(&r.shutdownWG, time.Minute); !success {
		r.logger.Warn("Task redispatcher timedout on shutdown.", tag.LifeCycleStopTimedout)
	}

	r.logger.Info("Task redispatcher stopped.", tag.LifeCycleStopped)
}

func (r *redispatcherImpl) AddTask(task Task) {
	priority := task.Priority()
	attempt := task.GetAttempt()

	r.Lock()
	defer r.Unlock()
	queue, ok := r.taskQueues[priority]
	if !ok {
		queue = make([]redispatchTask, 0)
	}
	r.taskQueues[priority] = append(queue, redispatchTask{
		task:           task,
		redispatchTime: r.getRedispatchTime(attempt),
	})

	r.setupTimerLocked()
}

func (r *redispatcherImpl) Redispatch(targetSize int) {
	doneCh := make(chan struct{})
	ntf := redispatchNotification{
		targetSize: targetSize,
		doneCh:     doneCh,
	}

	select {
	case r.redispatchCh <- ntf:
		// block until the redispatch is done
		<-doneCh
	case <-r.shutdownCh:
		close(doneCh)
		return
	}
}

func (r *redispatcherImpl) Size() int {
	r.Lock()
	defer r.Unlock()

	return r.sizeLocked()
}

func (r *redispatcherImpl) redispatchLoop() {
	defer r.shutdownWG.Done()

	for {
		select {
		case <-r.shutdownCh:
			return
		case notification := <-r.redispatchCh:
			r.redispatchTasks(notification)
		}
	}
}

func (r *redispatcherImpl) redispatchTasks(notification redispatchNotification) {
	r.Lock()
	defer r.Unlock()

	defer func() {
		if notification.doneCh != nil {
			close(notification.doneCh)
		}
		if r.sizeLocked() > 0 {
			// there are still tasks left in the queue, setup a redispatch timer for those tasks
			r.setupTimerLocked()
		}
	}()

	if r.isStopped() {
		return
	}

	queueSize := r.sizeLocked()
	r.metricsScope.RecordTimer(metrics.TaskRedispatchQueuePendingTasksTimer, time.Duration(queueSize))

	// add some buffer here as new tasks may be added
	targetRedispatched := queueSize + defaultBufferSize - notification.targetSize
	if targetRedispatched <= 0 {
		// target size has already been met, no need to redispatch
		return
	}

	totalRedispatched := 0
	now := r.timeSource.Now()
	for priority := range r.taskQueues {
		r.taskChFull[priority] = false
	}

	for priority, queue := range r.taskQueues {
		// sort by redispatch time
		sort.Slice(queue, func(i, j int) bool {
			return queue[i].redispatchTime.Before(queue[j].redispatchTime)
		})

		newStartIdx := 0
		for _, redispatchTask := range queue {
			if totalRedispatched >= targetRedispatched ||
				r.taskChFull[priority] ||
				redispatchTask.redispatchTime.After(now) {
				// Note the second condition regarding taskChFull is not 100% accurate
				// since task may get a new, lower priority upon redispatch, and
				// the taskCh for the new priority may not be full.
				// But the current estimation should be good enough as task with
				// lower priority should be executed after high priority ones,
				// so it's ok to leave them in the queue
				break
			}

			submitted, err := r.taskProcessor.TrySubmit(redispatchTask.task)
			if err != nil {
				if r.isStopped() {
					// if error is due to shard shutdown
					break
				}
				// otherwise it might be error from domain cache etc, add
				// the task to redispatch queue so that it can be retried
				r.logger.Error("Failed to redispatch task", tag.Error(err))
			}

			newStartIdx++ // task will be either redispatched or enqueued again at here
			newPriority := redispatchTask.task.Priority()
			if err != nil || !submitted {
				// failed to submit, enqueue again
				r.taskQueues[newPriority] = append(r.taskQueues[newPriority], redispatchTask)
			}
			if err == nil && !submitted {
				// task chan is full for the new priority
				r.taskChFull[newPriority] = true
			}
			if submitted {
				totalRedispatched++
			}
		}

		r.taskQueues[priority] = r.taskQueues[priority][newStartIdx:]

		if r.isStopped() {
			return
		}
	}
}

func (r *redispatcherImpl) setupTimerLocked() {
	if r.redispatchTimer != nil || r.isStopped() {
		return
	}

	r.redispatchTimer = time.AfterFunc(
		backoff.JitDuration(
			r.options.TaskRedispatchInterval(),
			r.options.TaskRedispatchIntervalJitterCoefficient(),
		),
		func() {
			r.Lock()
			defer r.Unlock()
			r.redispatchTimer = nil

			select {
			case r.redispatchCh <- redispatchNotification{
				targetSize: 0,
				doneCh:     nil,
			}:
			default:
			}
		},
	)
}

func (r *redispatcherImpl) sizeLocked() int {
	size := 0
	for _, queue := range r.taskQueues {
		size += len(queue)
	}

	return size
}

func (r *redispatcherImpl) isStopped() bool {
	return atomic.LoadInt32(&r.status) == common.DaemonStatusStopped
}

func (r *redispatcherImpl) getRedispatchTime(attempt int) time.Time {
	// note that elapsedTime (the first parameter) is not relevant when
	// the retry policy has not expiration interval
	return r.timeSource.Now().Add(r.backoffPolicy.ComputeNextDelay(0, attempt))
}
