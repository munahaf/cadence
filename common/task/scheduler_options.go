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
	"fmt"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/dynamicconfig"
)

type SchedulerOptions struct {
	SchedulerType        SchedulerType
	FIFOSchedulerOptions *FIFOTaskSchedulerOptions
	WRRSchedulerOptions  *WeightedRoundRobinTaskSchedulerOptions
}

func NewSchedulerOptions(
	schedulerType int,
	queueSize int,
	workerCount dynamicconfig.IntPropertyFn,
	dispatcherCount int,
	weights dynamicconfig.MapPropertyFn,
) (*SchedulerOptions, error) {
	options := &SchedulerOptions{
		SchedulerType: SchedulerType(schedulerType),
	}
	switch options.SchedulerType {
	case SchedulerTypeFIFO:
		options.FIFOSchedulerOptions = &FIFOTaskSchedulerOptions{
			QueueSize:       queueSize,
			WorkerCount:     workerCount,
			DispatcherCount: dispatcherCount,
			RetryPolicy:     common.CreateTaskProcessingRetryPolicy(),
		}
	case SchedulerTypeWRR:
		options.WRRSchedulerOptions = &WeightedRoundRobinTaskSchedulerOptions{
			Weights:         weights,
			QueueSize:       queueSize,
			WorkerCount:     workerCount,
			DispatcherCount: dispatcherCount,
			RetryPolicy:     common.CreateTaskProcessingRetryPolicy(),
		}
	default:
		return nil, fmt.Errorf("unknown task scheduler type: %v", schedulerType)
	}
	return options, nil
}

func (o *SchedulerOptions) String() string {
	return fmt.Sprintf("{schedulerType:%v, fifoSchedulerOptions:%s, wrrSchedulerOptions:%s}",
		o.SchedulerType, o.FIFOSchedulerOptions, o.WRRSchedulerOptions)
}
