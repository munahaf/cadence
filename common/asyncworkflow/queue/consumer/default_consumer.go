// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/yarpc"

	"github.com/uber/cadence/.gen/go/shared"
	"github.com/uber/cadence/.gen/go/sqlblobs"
	"github.com/uber/cadence/client/frontend"
	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/backoff"
	"github.com/uber/cadence/common/codec"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/messaging"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/types"
)

const (
	defaultShutdownTimeout = 5 * time.Second
	defaultConcurrency     = 100
)

type DefaultConsumer struct {
	queueID         string
	innerConsumer   messaging.Consumer
	logger          log.Logger
	scope           metrics.Scope
	frontendClient  frontend.Client
	ctx             context.Context
	cancelFn        context.CancelFunc
	wg              sync.WaitGroup
	shutdownTimeout time.Duration
	msgDecoder      codec.BinaryEncoder
	concurrency     int
}

type Option func(*DefaultConsumer)

func WithConcurrency(concurrency int) Option {
	return func(c *DefaultConsumer) {
		c.concurrency = concurrency
	}
}

func New(
	queueID string,
	innerConsumer messaging.Consumer,
	logger log.Logger,
	metricsClient metrics.Client,
	frontendClient frontend.Client,
	options ...Option,
) *DefaultConsumer {
	ctx, cancelFn := context.WithCancel(context.Background())
	c := &DefaultConsumer{
		queueID:         queueID,
		innerConsumer:   innerConsumer,
		logger:          logger.WithTags(tag.AsyncWFQueueID(queueID)),
		scope:           metricsClient.Scope(metrics.AsyncWorkflowConsumerScope),
		frontendClient:  frontendClient,
		ctx:             ctx,
		cancelFn:        cancelFn,
		shutdownTimeout: defaultShutdownTimeout,
		msgDecoder:      codec.NewThriftRWEncoder(),
		concurrency:     defaultConcurrency,
	}

	for _, opt := range options {
		opt(c)
	}

	return c
}

func (c *DefaultConsumer) Start() error {
	if err := c.innerConsumer.Start(); err != nil {
		return err
	}

	for i := 0; i < c.concurrency; i++ {
		c.wg.Add(1)
		go c.runProcessLoop()
		c.logger.Info("Started process loop", tag.Counter(i))
	}
	c.logger.Info("Started consumer", tag.Dynamic("concurrency", c.concurrency))
	return nil
}

func (c *DefaultConsumer) Stop() {
	c.logger.Info("Stopping consumer")
	c.cancelFn()
	c.wg.Wait()
	if !common.AwaitWaitGroup(&c.wg, c.shutdownTimeout) {
		c.logger.Warn("Consumer timed out on shutdown", tag.Dynamic("timeout", c.shutdownTimeout))
		return
	}

	c.innerConsumer.Stop()
	c.logger.Info("Stopped consumer")
}

func (c *DefaultConsumer) runProcessLoop() {
	defer c.wg.Done()

	for {
		select {
		case msg, ok := <-c.innerConsumer.Messages():
			if !ok {
				c.logger.Info("Consumer channel closed")
				return
			}

			c.processMessage(msg)
		case <-c.ctx.Done():
			c.logger.Info("Consumer context done so terminating loop")
			return
		}
	}
}

func (c *DefaultConsumer) processMessage(msg messaging.Message) {
	logger := c.logger.WithTags(tag.Dynamic("partition", msg.Partition()), tag.Dynamic("offset", msg.Offset()))
	logger.Debug("Received message")

	sw := c.scope.StartTimer(metrics.AsyncWorkflowProcessMsgLatency)
	defer sw.Stop()

	var request sqlblobs.AsyncRequestMessage
	if err := c.msgDecoder.Decode(msg.Value(), &request); err != nil {
		logger.Error("Failed to decode message", tag.Error(err))
		c.scope.IncCounter(metrics.AsyncWorkflowFailureCorruptMsgCount)
		if err := msg.Nack(); err != nil {
			logger.Error("Failed to nack message", tag.Error(err))
		}
		return
	}

	if err := c.processRequest(logger, &request); err != nil {
		logger.Error("Failed to process message", tag.Error(err))
		if err := msg.Nack(); err != nil {
			logger.Error("Failed to nack message", tag.Error(err))
		}
		return
	}

	if err := msg.Ack(); err != nil {
		logger.Error("Failed to ack message", tag.Error(err))
	}
	logger.Debug("Processed message successfully")
}

func (c *DefaultConsumer) processRequest(logger log.Logger, request *sqlblobs.AsyncRequestMessage) error {
	scope := c.scope.Tagged(metrics.AsyncWFRequestTypeTag(request.GetType().String()))
	switch request.GetType() {
	case sqlblobs.AsyncRequestTypeStartWorkflowExecutionAsyncRequest:
		startWFReq, err := decodeStartWorkflowRequest(request.GetPayload(), request.GetEncoding())
		if err != nil {
			scope.IncCounter(metrics.AsyncWorkflowFailureCorruptMsgCount)
			return err
		}

		yarpcCallOpts := getYARPCOptions(request.GetHeader())
		scope := scope.Tagged(metrics.DomainTag(startWFReq.GetDomain()))
		var resp *types.StartWorkflowExecutionResponse
		op := func() error {
			resp, err = c.frontendClient.StartWorkflowExecution(c.ctx, startWFReq, yarpcCallOpts...)
			return err
		}

		if err := callFrontendWithRetries(c.ctx, op); err != nil {
			scope.IncCounter(metrics.AsyncWorkflowFailureByFrontendCount)
			return fmt.Errorf("start workflow execution failed after all attempts: %w", err)
		}

		scope.IncCounter(metrics.AsyncWorkflowSuccessCount)
		logger.Info("StartWorkflowExecution succeeded", tag.WorkflowID(startWFReq.GetWorkflowID()), tag.WorkflowRunID(resp.GetRunID()))
	case sqlblobs.AsyncRequestTypeSignalWithStartWorkflowExecutionAsyncRequest:
		startWFReq, err := decodeSignalWithStartWorkflowRequest(request.GetPayload(), request.GetEncoding())
		if err != nil {
			c.scope.IncCounter(metrics.AsyncWorkflowFailureCorruptMsgCount)
			return err
		}

		yarpcCallOpts := getYARPCOptions(request.GetHeader())
		scope := c.scope.Tagged(metrics.DomainTag(startWFReq.GetDomain()))
		var resp *types.StartWorkflowExecutionResponse
		op := func() error {
			resp, err = c.frontendClient.SignalWithStartWorkflowExecution(c.ctx, startWFReq, yarpcCallOpts...)
			return err
		}

		if err := callFrontendWithRetries(c.ctx, op); err != nil {
			scope.IncCounter(metrics.AsyncWorkflowFailureByFrontendCount)
			return fmt.Errorf("signal with start workflow execution failed after all attempts: %w", err)
		}

		scope.IncCounter(metrics.AsyncWorkflowSuccessCount)
		logger.Info("SignalWithStartWorkflowExecution succeeded", tag.WorkflowID(startWFReq.GetWorkflowID()), tag.WorkflowRunID(resp.GetRunID()))
	default:
		c.scope.IncCounter(metrics.AsyncWorkflowFailureCorruptMsgCount)
		return &UnsupportedRequestType{Type: request.GetType()}
	}

	return nil
}

func callFrontendWithRetries(ctx context.Context, op func() error) error {
	throttleRetry := backoff.NewThrottleRetry(
		backoff.WithRetryPolicy(common.CreateFrontendServiceRetryPolicy()),
		backoff.WithRetryableError(common.IsServiceTransientError),
	)

	return throttleRetry.Do(ctx, op)
}

func getYARPCOptions(header *shared.Header) []yarpc.CallOption {
	if header == nil || header.GetFields() == nil {
		return nil
	}

	// sort the header fields to make the tests deterministic
	fields := header.GetFields()
	sortedKeys := make([]string, 0, len(fields))
	for k := range fields {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	var opts []yarpc.CallOption
	for _, k := range sortedKeys {
		opts = append(opts, yarpc.WithHeader(k, string(fields[k])))
	}
	return opts
}

func decodeStartWorkflowRequest(payload []byte, encoding string) (*types.StartWorkflowExecutionRequest, error) {
	if encoding != string(common.EncodingTypeJSON) {
		return nil, &UnsupportedEncoding{EncodingType: encoding}
	}

	var startRequest types.StartWorkflowExecutionAsyncRequest
	if err := json.Unmarshal(payload, &startRequest); err != nil {
		return nil, err
	}
	return startRequest.StartWorkflowExecutionRequest, nil
}

func decodeSignalWithStartWorkflowRequest(payload []byte, encoding string) (*types.SignalWithStartWorkflowExecutionRequest, error) {
	if encoding != string(common.EncodingTypeJSON) {
		return nil, &UnsupportedEncoding{EncodingType: encoding}
	}

	var startRequest types.SignalWithStartWorkflowExecutionAsyncRequest
	if err := json.Unmarshal(payload, &startRequest); err != nil {
		return nil, err
	}
	return startRequest.SignalWithStartWorkflowExecutionRequest, nil
}
