// Copyright (c) 2021 Uber Technologies Inc.
//
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

package thrift

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/common/types/testdata"
)

func TestHistoryTerminateWorkflowExecutionRequestConversion(t *testing.T) {
	for _, item := range []*types.HistoryTerminateWorkflowExecutionRequest{nil, {}, &testdata.HistoryTerminateWorkflowExecutionRequest} {
		assert.Equal(t, item, ToHistoryTerminateWorkflowExecutionRequest(FromHistoryTerminateWorkflowExecutionRequest(item)))
	}
}

func TestDescribeMutableStateRequestConversion(t *testing.T) {
	testCases := []*types.DescribeMutableStateRequest{
		nil,
		{},
		&testdata.HistoryDescribeMutableStateRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryDescribeMutableStateRequest(original)
		roundTripObj := ToHistoryDescribeMutableStateRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestDescribeMutableStateResponseConversion(t *testing.T) {
	testCases := []*types.DescribeMutableStateResponse{
		nil,
		{},
		&testdata.HistoryDescribeMutableStateResponse,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryDescribeMutableStateResponse(original)
		roundTripObj := ToHistoryDescribeMutableStateResponse(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryDescribeWorkflowExecutionRequestConversion(t *testing.T) {
	testCases := []*types.HistoryDescribeWorkflowExecutionRequest{
		nil,
		{},
		&testdata.HistoryDescribeWorkflowExecutionRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryDescribeWorkflowExecutionRequest(original)
		roundTripObj := ToHistoryDescribeWorkflowExecutionRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestDomainFilterConversion(t *testing.T) {
	testCases := []*types.DomainFilter{
		nil,
		{},
		{DomainIDs: []string{"test"}, ReverseMatch: true},
	}

	for _, original := range testCases {
		thriftObj := FromDomainFilter(original)
		roundTripObj := ToDomainFilter(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestEventAlreadyStartedErrorConversion(t *testing.T) {
	testCases := []*types.EventAlreadyStartedError{
		nil,
		{},
		&testdata.EventAlreadyStartedError,
	}

	for _, original := range testCases {
		thriftObj := FromEventAlreadyStartedError(original)
		roundTripObj := ToEventAlreadyStartedError(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestFailoverMarkerTokenConversion(t *testing.T) {
	testCases := []*types.FailoverMarkerToken{
		nil,
		{},
		&testdata.FailoverMarkerToken,
	}

	for _, original := range testCases {
		thriftObj := FromFailoverMarkerToken(original)
		roundTripObj := ToFailoverMarkerToken(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestGetMutableStateRequestConversion(t *testing.T) {
	testCases := []*types.GetMutableStateRequest{
		nil,
		{},
		&testdata.HistoryGetMutableStateRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryGetMutableStateRequest(original)
		roundTripObj := ToHistoryGetMutableStateRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestGetMutableStateResponseConversion(t *testing.T) {
	testCases := []*types.GetMutableStateResponse{
		nil,
		{},
		&testdata.HistoryGetMutableStateResponse,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryGetMutableStateResponse(original)
		roundTripObj := ToHistoryGetMutableStateResponse(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestNotifyFailoverMarkersRequestConversion(t *testing.T) {
	testCases := []*types.NotifyFailoverMarkersRequest{
		nil,
		{},
		{FailoverMarkerTokens: []*types.FailoverMarkerToken{&testdata.FailoverMarkerToken}},
	}

	for _, original := range testCases {
		thriftObj := FromHistoryNotifyFailoverMarkersRequest(original)
		roundTripObj := ToHistoryNotifyFailoverMarkersRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestParentExecutionInfoConversion(t *testing.T) {
	testCases := []*types.ParentExecutionInfo{
		nil,
		{},
		&testdata.ParentExecutionInfo,
	}

	for _, original := range testCases {
		thriftObj := FromParentExecutionInfo(original)
		roundTripObj := ToParentExecutionInfo(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestPollMutableStateRequestConversion(t *testing.T) {
	testCases := []*types.PollMutableStateRequest{
		nil,
		{},
		&testdata.HistoryPollMutableStateRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryPollMutableStateRequest(original)
		roundTripObj := ToHistoryPollMutableStateRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestPollMutableStateResponseConversion(t *testing.T) {
	testCases := []*types.PollMutableStateResponse{
		nil,
		{},
		&testdata.HistoryPollMutableStateResponse,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryPollMutableStateResponse(original)
		roundTripObj := ToHistoryPollMutableStateResponse(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestProcessingQueueStateConversion(t *testing.T) {
	testCases := []*types.ProcessingQueueState{
		nil,
		{},
		{Level: common.Int32Ptr(1), AckLevel: common.Int64Ptr(1), MaxLevel: common.Int64Ptr(1), DomainFilter: &types.DomainFilter{DomainIDs: []string{"test"}, ReverseMatch: true}},
	}

	for _, original := range testCases {
		thriftObj := FromProcessingQueueState(original)
		roundTripObj := ToProcessingQueueState(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestProcessingQueueStatesConversion(t *testing.T) {
	testCases := []*types.ProcessingQueueStates{
		nil,
		{},
		{
			StatesByCluster: map[string][]*types.ProcessingQueueState{
				"test": {
					{Level: common.Int32Ptr(1), AckLevel: common.Int64Ptr(1), MaxLevel: common.Int64Ptr(1), DomainFilter: &types.DomainFilter{DomainIDs: []string{"test"}, ReverseMatch: true}},
				},
			},
		},
	}

	for _, original := range testCases {
		thriftObj := FromProcessingQueueStates(original)
		roundTripObj := ToProcessingQueueStates(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryQueryWorkflowRequestConversion(t *testing.T) {
	testCases := []*types.HistoryQueryWorkflowRequest{
		nil,
		{},
		&testdata.HistoryQueryWorkflowRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryQueryWorkflowRequest(original)
		roundTripObj := ToHistoryQueryWorkflowRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryQueryWorkflowResponseConversion(t *testing.T) {
	testCases := []*types.HistoryQueryWorkflowResponse{
		nil,
		{},
		&testdata.HistoryQueryWorkflowResponse,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryQueryWorkflowResponse(original)
		roundTripObj := ToHistoryQueryWorkflowResponse(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryReapplyEventsRequestConversion(t *testing.T) {
	testCases := []*types.HistoryReapplyEventsRequest{
		nil,
		{},
		&testdata.HistoryReapplyEventsRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryReapplyEventsRequest(original)
		roundTripObj := ToHistoryReapplyEventsRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}
func TestHistoryRecordActivityTaskHeartbeatRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRecordActivityTaskHeartbeatRequest{
		nil,
		{},
		&testdata.HistoryRecordActivityTaskHeartbeatRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRecordActivityTaskHeartbeatRequest(original)
		roundTripObj := ToHistoryRecordActivityTaskHeartbeatRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestRecordActivityTaskStartedRequestConversion(t *testing.T) {
	testCases := []*types.RecordActivityTaskStartedRequest{
		nil,
		{},
		&testdata.HistoryRecordActivityTaskStartedRequest,
	}

	for _, original := range testCases {
		thriftObj := FromRecordActivityTaskStartedRequest(original)
		roundTripObj := ToRecordActivityTaskStartedRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestRecordActivityTaskStartedResponseConversion(t *testing.T) {
	testCases := []*types.RecordActivityTaskStartedResponse{
		nil,
		{},
		&testdata.HistoryRecordActivityTaskStartedResponse,
	}

	for _, original := range testCases {
		thriftObj := FromRecordActivityTaskStartedResponse(original)
		roundTripObj := ToRecordActivityTaskStartedResponse(thriftObj)
		opt := cmpopts.IgnoreFields(types.WorkflowExecutionStartedEventAttributes{}, "ParentWorkflowDomainID")
		if diff := cmp.Diff(original, roundTripObj, opt); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestRecordChildExecutionCompletedRequestConversion(t *testing.T) {
	testCases := []*types.RecordChildExecutionCompletedRequest{
		nil,
		{},
		&testdata.HistoryRecordChildExecutionCompletedRequest,
	}

	for _, original := range testCases {
		thriftObj := FromRecordChildExecutionCompletedRequest(original)
		roundTripObj := ToRecordChildExecutionCompletedRequest(thriftObj)
		opt := cmpopts.IgnoreFields(types.WorkflowExecutionStartedEventAttributes{}, "ParentWorkflowDomainID")
		if diff := cmp.Diff(original, roundTripObj, opt); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestRecordDecisionTaskStartedRequestConversion(t *testing.T) {
	testCases := []*types.RecordDecisionTaskStartedRequest{
		nil,
		{},
		&testdata.HistoryRecordDecisionTaskStartedRequest,
	}

	for _, original := range testCases {
		thriftObj := FromRecordDecisionTaskStartedRequest(original)
		roundTripObj := ToRecordDecisionTaskStartedRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestRecordDecisionTaskStartedResponseConversion(t *testing.T) {
	testCases := []*types.RecordDecisionTaskStartedResponse{
		nil,
		{},
		&testdata.HistoryRecordDecisionTaskStartedResponse,
	}

	for _, original := range testCases {
		thriftObj := FromRecordDecisionTaskStartedResponse(original)
		roundTripObj := ToRecordDecisionTaskStartedResponse(thriftObj)
		opt := cmpopts.IgnoreFields(types.WorkflowExecutionStartedEventAttributes{}, "ParentWorkflowDomainID")
		if diff := cmp.Diff(original, roundTripObj, opt); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestHistoryRefreshWorkflowTasksRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRefreshWorkflowTasksRequest{
		nil,
		{},
		&testdata.HistoryRefreshWorkflowTasksRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRefreshWorkflowTasksRequest(original)
		roundTripObj := ToHistoryRefreshWorkflowTasksRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestRemoveSignalMutableStateRequestConversion(t *testing.T) {
	testCases := []*types.RemoveSignalMutableStateRequest{
		nil,
		{},
		{DomainUUID: "test-uuid", WorkflowExecution: &types.WorkflowExecution{WorkflowID: "test", RunID: "test"}, RequestID: "test-req"},
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRemoveSignalMutableStateRequest(original)
		roundTripObj := ToHistoryRemoveSignalMutableStateRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestReplicateEventsV2RequestConversion(t *testing.T) {
	testCases := []*types.ReplicateEventsV2Request{
		nil,
		{},
		&testdata.HistoryReplicateEventsV2Request,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryReplicateEventsV2Request(original)
		roundTripObj := ToHistoryReplicateEventsV2Request(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryRequestCancelWorkflowExecutionRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRequestCancelWorkflowExecutionRequest{
		nil,
		{},
		&testdata.HistoryRequestCancelWorkflowExecutionRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRequestCancelWorkflowExecutionRequest(original)
		roundTripObj := ToHistoryRequestCancelWorkflowExecutionRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryResetStickyTaskListRequestConversion(t *testing.T) {
	testCases := []*types.HistoryResetStickyTaskListRequest{
		nil,
		{},
		&testdata.HistoryResetStickyTaskListRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryResetStickyTaskListRequest(original)
		roundTripObj := ToHistoryResetStickyTaskListRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryResetStickyTaskListResponseConversion(t *testing.T) {
	testCases := []*types.HistoryResetStickyTaskListResponse{
		nil,
		{},
	}

	for _, original := range testCases {
		thriftObj := FromHistoryResetStickyTaskListResponse(original)
		roundTripObj := ToHistoryResetStickyTaskListResponse(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryResetWorkflowExecutionRequestConversion(t *testing.T) {
	testCases := []*types.HistoryResetWorkflowExecutionRequest{
		nil,
		{},
		&testdata.HistoryResetWorkflowExecutionRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryResetWorkflowExecutionRequest(original)
		roundTripObj := ToHistoryResetWorkflowExecutionRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryRespondActivityTaskCanceledRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRespondActivityTaskCanceledRequest{
		nil,
		{},
		&testdata.HistoryRespondActivityTaskCanceledRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRespondActivityTaskCanceledRequest(original)
		roundTripObj := ToHistoryRespondActivityTaskCanceledRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryRespondActivityTaskCompletedRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRespondActivityTaskCompletedRequest{
		nil,
		{},
		&testdata.HistoryRespondActivityTaskCompletedRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRespondActivityTaskCompletedRequest(original)
		roundTripObj := ToHistoryRespondActivityTaskCompletedRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryRespondActivityTaskFailedRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRespondActivityTaskFailedRequest{
		nil,
		{},
		&testdata.HistoryRespondActivityTaskFailedRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRespondActivityTaskFailedRequest(original)
		roundTripObj := ToHistoryRespondActivityTaskFailedRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryRespondDecisionTaskCompletedRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRespondDecisionTaskCompletedRequest{
		nil,
		{},
		&testdata.HistoryRespondDecisionTaskCompletedRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRespondDecisionTaskCompletedRequest(original)
		roundTripObj := ToHistoryRespondDecisionTaskCompletedRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryRespondDecisionTaskCompletedResponseConversion(t *testing.T) {
	testCases := []*types.HistoryRespondDecisionTaskCompletedResponse{
		nil,
		{},
		&testdata.HistoryRespondDecisionTaskCompletedResponse,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRespondDecisionTaskCompletedResponse(original)
		roundTripObj := ToHistoryRespondDecisionTaskCompletedResponse(thriftObj)
		opt := cmpopts.IgnoreFields(types.WorkflowExecutionStartedEventAttributes{}, "ParentWorkflowDomainID")
		if diff := cmp.Diff(original, roundTripObj, opt); diff != "" {
			t.Fatalf("Mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestHistoryRespondDecisionTaskFailedRequestConversion(t *testing.T) {
	testCases := []*types.HistoryRespondDecisionTaskFailedRequest{
		nil,
		{},
		&testdata.HistoryRespondDecisionTaskFailedRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryRespondDecisionTaskFailedRequest(original)
		roundTripObj := ToHistoryRespondDecisionTaskFailedRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestScheduleDecisionTaskRequestConversion(t *testing.T) {
	testCases := []*types.ScheduleDecisionTaskRequest{
		nil,
		{},
		&testdata.HistoryScheduleDecisionTaskRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryScheduleDecisionTaskRequest(original)
		roundTripObj := ToHistoryScheduleDecisionTaskRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestShardOwnershipLostErrorConversion(t *testing.T) {
	testCases := []*types.ShardOwnershipLostError{
		nil,
		{},
		&testdata.ShardOwnershipLostError,
	}

	for _, original := range testCases {
		thriftObj := FromShardOwnershipLostError(original)
		roundTripObj := ToShardOwnershipLostError(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistorySignalWithStartWorkflowExecutionRequestConversion(t *testing.T) {
	testCases := []*types.HistorySignalWithStartWorkflowExecutionRequest{
		nil,
		{},
		&testdata.HistorySignalWithStartWorkflowExecutionRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistorySignalWithStartWorkflowExecutionRequest(original)
		roundTripObj := ToHistorySignalWithStartWorkflowExecutionRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistorySignalWorkflowExecutionRequestConversion(t *testing.T) {
	testCases := []*types.HistorySignalWorkflowExecutionRequest{
		nil,
		{},
		&testdata.HistorySignalWorkflowExecutionRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistorySignalWorkflowExecutionRequest(original)
		roundTripObj := ToHistorySignalWorkflowExecutionRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestHistoryStartWorkflowExecutionRequestConversion(t *testing.T) {
	testCases := []*types.HistoryStartWorkflowExecutionRequest{
		nil,
		{},
		&testdata.HistoryStartWorkflowExecutionRequest,
	}

	for _, original := range testCases {
		thriftObj := FromHistoryStartWorkflowExecutionRequest(original)
		roundTripObj := ToHistoryStartWorkflowExecutionRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestSyncActivityRequestConversion(t *testing.T) {
	testCases := []*types.SyncActivityRequest{
		nil,
		{},
		{
			DomainID:           "test-domain-id",
			WorkflowID:         "test-workflow-id",
			RunID:              "test-run-id",
			Version:            1,
			ScheduledID:        1,
			ScheduledTime:      common.Int64Ptr(98765),
			StartedID:          1,
			StartedTime:        common.Int64Ptr(98765),
			LastHeartbeatTime:  common.Int64Ptr(98765),
			Details:            []byte("test-details"),
			Attempt:            3,
			LastFailureReason:  common.StringPtr("test-last-failure-reason"),
			LastWorkerIdentity: "test-last-worker-identity",
			LastFailureDetails: []byte("test-last-failure-details"),
			VersionHistory:     &testdata.VersionHistory,
		},
	}

	for _, original := range testCases {
		thriftObj := FromHistorySyncActivityRequest(original)
		roundTripObj := ToHistorySyncActivityRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}

func TestSyncShardStatusRequestConversion(t *testing.T) {
	testCases := []*types.SyncShardStatusRequest{
		nil,
		{},
		{
			SourceCluster: "test-source-cluster",
			ShardID:       1,
			Timestamp:     common.Int64Ptr(98765),
		},
	}

	for _, original := range testCases {
		thriftObj := FromHistorySyncShardStatusRequest(original)
		roundTripObj := ToHistorySyncShardStatusRequest(thriftObj)
		assert.Equal(t, original, roundTripObj)
	}
}
