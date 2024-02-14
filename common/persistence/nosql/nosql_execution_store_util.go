// Copyright (c) 2017-2021 Uber Technologies, Inc.
// Portions of the Software are attributed to Copyright (c) 2020 Temporal Technologies Inc.
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

package nosql

import (
	"context"
	"fmt"
	"time"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/checksum"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/nosql/nosqlplugin"
	"github.com/uber/cadence/common/types"
)

func (d *nosqlExecutionStore) prepareCreateWorkflowExecutionRequestWithMaps(newWorkflow *persistence.InternalWorkflowSnapshot) (*nosqlplugin.WorkflowExecutionRequest, error) {
	executionInfo := newWorkflow.ExecutionInfo
	lastWriteVersion := newWorkflow.LastWriteVersion
	checkSum := newWorkflow.Checksum
	versionHistories := newWorkflow.VersionHistories
	nowTimestamp := time.Now()

	executionRequest, err := d.prepareCreateWorkflowExecutionTxn(
		executionInfo, versionHistories, checkSum,
		nowTimestamp, lastWriteVersion,
	)
	if err != nil {
		return nil, err
	}

	executionRequest.ActivityInfos, err = d.prepareActivityInfosForWorkflowTxn(newWorkflow.ActivityInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.TimerInfos, err = d.prepareTimerInfosForWorkflowTxn(newWorkflow.TimerInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.ChildWorkflowInfos, err = d.prepareChildWFInfosForWorkflowTxn(newWorkflow.ChildExecutionInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.RequestCancelInfos, err = d.prepareRequestCancelsForWorkflowTxn(newWorkflow.RequestCancelInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.SignalInfos, err = d.prepareSignalInfosForWorkflowTxn(newWorkflow.SignalInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.SignalRequestedIDs = newWorkflow.SignalRequestedIDs
	executionRequest.MapsWriteMode = nosqlplugin.WorkflowExecutionMapsWriteModeCreate
	return executionRequest, nil
}

func (d *nosqlExecutionStore) prepareResetWorkflowExecutionRequestWithMapsAndEventBuffer(resetWorkflow *persistence.InternalWorkflowSnapshot) (*nosqlplugin.WorkflowExecutionRequest, error) {
	executionInfo := resetWorkflow.ExecutionInfo
	lastWriteVersion := resetWorkflow.LastWriteVersion
	checkSum := resetWorkflow.Checksum
	versionHistories := resetWorkflow.VersionHistories
	nowTimestamp := time.Now()

	executionRequest, err := d.prepareUpdateWorkflowExecutionTxn(
		executionInfo, versionHistories, checkSum,
		nowTimestamp, lastWriteVersion,
	)
	if err != nil {
		return nil, err
	}
	//reset 6 maps
	executionRequest.ActivityInfos, err = d.prepareActivityInfosForWorkflowTxn(resetWorkflow.ActivityInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.TimerInfos, err = d.prepareTimerInfosForWorkflowTxn(resetWorkflow.TimerInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.ChildWorkflowInfos, err = d.prepareChildWFInfosForWorkflowTxn(resetWorkflow.ChildExecutionInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.RequestCancelInfos, err = d.prepareRequestCancelsForWorkflowTxn(resetWorkflow.RequestCancelInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.SignalInfos, err = d.prepareSignalInfosForWorkflowTxn(resetWorkflow.SignalInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.SignalRequestedIDs = resetWorkflow.SignalRequestedIDs
	executionRequest.MapsWriteMode = nosqlplugin.WorkflowExecutionMapsWriteModeReset
	// delete buffered events
	executionRequest.EventBufferWriteMode = nosqlplugin.EventBufferWriteModeClear
	// condition
	executionRequest.PreviousNextEventIDCondition = &resetWorkflow.Condition
	return executionRequest, nil
}

func (d *nosqlExecutionStore) prepareUpdateWorkflowExecutionRequestWithMapsAndEventBuffer(workflowMutation *persistence.InternalWorkflowMutation) (*nosqlplugin.WorkflowExecutionRequest, error) {
	executionInfo := workflowMutation.ExecutionInfo
	lastWriteVersion := workflowMutation.LastWriteVersion
	checkSum := workflowMutation.Checksum
	versionHistories := workflowMutation.VersionHistories
	nowTimestamp := time.Now()

	executionRequest, err := d.prepareUpdateWorkflowExecutionTxn(
		executionInfo, versionHistories, checkSum,
		nowTimestamp, lastWriteVersion,
	)
	if err != nil {
		return nil, err
	}

	// merge 6 maps
	executionRequest.ActivityInfos, err = d.prepareActivityInfosForWorkflowTxn(workflowMutation.UpsertActivityInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.TimerInfos, err = d.prepareTimerInfosForWorkflowTxn(workflowMutation.UpsertTimerInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.ChildWorkflowInfos, err = d.prepareChildWFInfosForWorkflowTxn(workflowMutation.UpsertChildExecutionInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.RequestCancelInfos, err = d.prepareRequestCancelsForWorkflowTxn(workflowMutation.UpsertRequestCancelInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.SignalInfos, err = d.prepareSignalInfosForWorkflowTxn(workflowMutation.UpsertSignalInfos)
	if err != nil {
		return nil, err
	}
	executionRequest.SignalRequestedIDs = workflowMutation.UpsertSignalRequestedIDs

	// delete from 6 maps
	executionRequest.ActivityInfoKeysToDelete = workflowMutation.DeleteActivityInfos
	executionRequest.TimerInfoKeysToDelete = workflowMutation.DeleteTimerInfos
	executionRequest.ChildWorkflowInfoKeysToDelete = workflowMutation.DeleteChildExecutionInfos
	executionRequest.RequestCancelInfoKeysToDelete = workflowMutation.DeleteRequestCancelInfos
	executionRequest.SignalInfoKeysToDelete = workflowMutation.DeleteSignalInfos
	executionRequest.SignalRequestedIDsKeysToDelete = workflowMutation.DeleteSignalRequestedIDs

	// map write mode
	executionRequest.MapsWriteMode = nosqlplugin.WorkflowExecutionMapsWriteModeUpdate

	// prepare to write buffer event
	executionRequest.EventBufferWriteMode = nosqlplugin.EventBufferWriteModeNone
	if workflowMutation.ClearBufferedEvents {
		executionRequest.EventBufferWriteMode = nosqlplugin.EventBufferWriteModeClear
	} else if workflowMutation.NewBufferedEvents != nil {
		executionRequest.EventBufferWriteMode = nosqlplugin.EventBufferWriteModeAppend
		executionRequest.NewBufferedEventBatch = workflowMutation.NewBufferedEvents
	}

	// condition
	executionRequest.PreviousNextEventIDCondition = &workflowMutation.Condition
	return executionRequest, nil
}

func (d *nosqlExecutionStore) prepareTimerTasksForWorkflowTxn(domainID, workflowID, runID string, timerTasks []persistence.Task) ([]*nosqlplugin.TimerTask, error) {
	var tasks []*nosqlplugin.TimerTask

	for _, task := range timerTasks {
		var eventID int64
		var attempt int64

		timeoutType := 0

		switch t := task.(type) {
		case *persistence.DecisionTimeoutTask:
			eventID = t.EventID
			timeoutType = t.TimeoutType
			attempt = t.ScheduleAttempt

		case *persistence.ActivityTimeoutTask:
			eventID = t.EventID
			timeoutType = t.TimeoutType
			attempt = t.Attempt

		case *persistence.UserTimerTask:
			eventID = t.EventID

		case *persistence.ActivityRetryTimerTask:
			eventID = t.EventID
			attempt = int64(t.Attempt)

		case *persistence.WorkflowBackoffTimerTask:
			eventID = t.EventID
			timeoutType = t.TimeoutType

		case *persistence.WorkflowTimeoutTask:
			// noop

		case *persistence.DeleteHistoryEventTask:
			// noop

		default:
			return nil, &types.InternalServiceError{
				Message: fmt.Sprintf("Unknow timer type: %v", task.GetType()),
			}
		}

		nt := &nosqlplugin.TimerTask{
			TaskType:   task.GetType(),
			DomainID:   domainID,
			WorkflowID: workflowID,
			RunID:      runID,

			VisibilityTimestamp: task.GetVisibilityTimestamp(),
			TaskID:              task.GetTaskID(),

			TimeoutType:     timeoutType,
			EventID:         eventID,
			ScheduleAttempt: attempt,
			Version:         task.GetVersion(),
		}
		tasks = append(tasks, nt)
	}

	return tasks, nil
}

func (d *nosqlExecutionStore) prepareReplicationTasksForWorkflowTxn(domainID, workflowID, runID string, replicationTasks []persistence.Task) ([]*nosqlplugin.ReplicationTask, error) {
	var tasks []*nosqlplugin.ReplicationTask

	for _, task := range replicationTasks {
		// Replication task specific information
		firstEventID := common.EmptyEventID
		nextEventID := common.EmptyEventID
		version := common.EmptyVersion //nolint:ineffassign
		activityScheduleID := common.EmptyEventID
		var branchToken, newRunBranchToken []byte

		switch task.GetType() {
		case persistence.ReplicationTaskTypeHistory:
			histTask := task.(*persistence.HistoryReplicationTask)
			branchToken = histTask.BranchToken
			newRunBranchToken = histTask.NewRunBranchToken
			firstEventID = histTask.FirstEventID
			nextEventID = histTask.NextEventID
			version = task.GetVersion()

		case persistence.ReplicationTaskTypeSyncActivity:
			version = task.GetVersion()
			activityScheduleID = task.(*persistence.SyncActivityTask).ScheduledID

		case persistence.ReplicationTaskTypeFailoverMarker:
			version = task.GetVersion()

		default:
			return nil, &types.InternalServiceError{
				Message: fmt.Sprintf("Unknown replication type: %v", task.GetType()),
			}
		}

		nt := &nosqlplugin.ReplicationTask{
			TaskType:          task.GetType(),
			DomainID:          domainID,
			WorkflowID:        workflowID,
			RunID:             runID,
			CreationTime:      task.GetVisibilityTimestamp(),
			TaskID:            task.GetTaskID(),
			FirstEventID:      firstEventID,
			NextEventID:       nextEventID,
			Version:           version,
			ScheduledID:       activityScheduleID,
			BranchToken:       branchToken,
			NewRunBranchToken: newRunBranchToken,
		}
		tasks = append(tasks, nt)
	}

	return tasks, nil
}

func (d *nosqlExecutionStore) prepareCrossClusterTasksForWorkflowTxn(domainID, workflowID, runID string, crossClusterTasks []persistence.Task) ([]*nosqlplugin.CrossClusterTask, error) {
	var tasks []*nosqlplugin.CrossClusterTask

	for _, task := range crossClusterTasks {
		var taskList string
		var scheduleID int64
		var targetCluster string
		targetDomainID := domainID // default to source domain, can't be empty, since empty string is not valid UUID
		targetDomainIDs := map[string]struct{}{}
		var targetWorkflowID string
		targetRunID := persistence.CrossClusterTaskDefaultTargetRunID
		targetChildWorkflowOnly := false
		recordVisibility := false

		switch task.GetType() {
		case persistence.CrossClusterTaskTypeStartChildExecution:
			targetCluster = task.(*persistence.CrossClusterStartChildExecutionTask).TargetCluster
			targetDomainID = task.(*persistence.CrossClusterStartChildExecutionTask).TargetDomainID
			targetWorkflowID = task.(*persistence.CrossClusterStartChildExecutionTask).TargetWorkflowID
			scheduleID = task.(*persistence.CrossClusterStartChildExecutionTask).InitiatedID

		case persistence.CrossClusterTaskTypeCancelExecution:
			targetCluster = task.(*persistence.CrossClusterCancelExecutionTask).TargetCluster
			targetDomainID = task.(*persistence.CrossClusterCancelExecutionTask).TargetDomainID
			targetWorkflowID = task.(*persistence.CrossClusterCancelExecutionTask).TargetWorkflowID
			targetRunID = task.(*persistence.CrossClusterCancelExecutionTask).TargetRunID
			if targetRunID == "" {
				targetRunID = persistence.CrossClusterTaskDefaultTargetRunID
			}
			targetChildWorkflowOnly = task.(*persistence.CrossClusterCancelExecutionTask).TargetChildWorkflowOnly
			scheduleID = task.(*persistence.CrossClusterCancelExecutionTask).InitiatedID

		case persistence.CrossClusterTaskTypeSignalExecution:
			targetCluster = task.(*persistence.CrossClusterSignalExecutionTask).TargetCluster
			targetDomainID = task.(*persistence.CrossClusterSignalExecutionTask).TargetDomainID
			targetWorkflowID = task.(*persistence.CrossClusterSignalExecutionTask).TargetWorkflowID
			targetRunID = task.(*persistence.CrossClusterSignalExecutionTask).TargetRunID
			if targetRunID == "" {
				targetRunID = persistence.CrossClusterTaskDefaultTargetRunID
			}
			targetChildWorkflowOnly = task.(*persistence.CrossClusterSignalExecutionTask).TargetChildWorkflowOnly
			scheduleID = task.(*persistence.CrossClusterSignalExecutionTask).InitiatedID

		case persistence.CrossClusterTaskTypeRecordChildExeuctionCompleted:
			targetCluster = task.(*persistence.CrossClusterRecordChildExecutionCompletedTask).TargetCluster
			targetDomainID = task.(*persistence.CrossClusterRecordChildExecutionCompletedTask).TargetDomainID
			targetWorkflowID = task.(*persistence.CrossClusterRecordChildExecutionCompletedTask).TargetWorkflowID
			targetRunID = task.(*persistence.CrossClusterRecordChildExecutionCompletedTask).TargetRunID
			if targetRunID == "" {
				targetRunID = persistence.CrossClusterTaskDefaultTargetRunID
			}

		case persistence.CrossClusterTaskTypeApplyParentClosePolicy:
			targetCluster = task.(*persistence.CrossClusterApplyParentClosePolicyTask).TargetCluster
			targetDomainIDs = task.(*persistence.CrossClusterApplyParentClosePolicyTask).TargetDomainIDs

		default:
			return nil, &types.InternalServiceError{
				Message: fmt.Sprintf("Unknown cross-cluster task type: %v", task.GetType()),
			}
		}

		nt := &nosqlplugin.CrossClusterTask{
			TransferTask: nosqlplugin.TransferTask{
				TaskType:                task.GetType(),
				DomainID:                domainID,
				WorkflowID:              workflowID,
				RunID:                   runID,
				VisibilityTimestamp:     task.GetVisibilityTimestamp(),
				TaskID:                  task.GetTaskID(),
				TargetDomainID:          targetDomainID,
				TargetDomainIDs:         targetDomainIDs,
				TargetWorkflowID:        targetWorkflowID,
				TargetRunID:             targetRunID,
				TargetChildWorkflowOnly: targetChildWorkflowOnly,
				TaskList:                taskList,
				ScheduleID:              scheduleID,
				RecordVisibility:        recordVisibility,
				Version:                 task.GetVersion(),
			},
			TargetCluster: targetCluster,
		}
		tasks = append(tasks, nt)
	}

	return tasks, nil
}

func (d *nosqlExecutionStore) prepareNoSQLTasksForWorkflowTxn(
	domainID, workflowID, runID string,
	persistenceTransferTasks, persistenceCrossClusterTasks, persistenceReplicationTasks, persistenceTimerTasks []persistence.Task,
	transferTasksToAppend []*nosqlplugin.TransferTask,
	crossClusterTasksToAppend []*nosqlplugin.CrossClusterTask,
	replicationTasksToAppend []*nosqlplugin.ReplicationTask,
	timerTasksToAppend []*nosqlplugin.TimerTask,
) ([]*nosqlplugin.TransferTask, []*nosqlplugin.CrossClusterTask, []*nosqlplugin.ReplicationTask, []*nosqlplugin.TimerTask, error) {
	transferTasks, err := d.prepareTransferTasksForWorkflowTxn(domainID, workflowID, runID, persistenceTransferTasks)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	transferTasksToAppend = append(transferTasksToAppend, transferTasks...)

	crossClusterTasks, err := d.prepareCrossClusterTasksForWorkflowTxn(domainID, workflowID, runID, persistenceCrossClusterTasks)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	crossClusterTasksToAppend = append(crossClusterTasksToAppend, crossClusterTasks...)

	replicationTasks, err := d.prepareReplicationTasksForWorkflowTxn(domainID, workflowID, runID, persistenceReplicationTasks)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	replicationTasksToAppend = append(replicationTasksToAppend, replicationTasks...)

	timerTasks, err := d.prepareTimerTasksForWorkflowTxn(domainID, workflowID, runID, persistenceTimerTasks)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	timerTasksToAppend = append(timerTasksToAppend, timerTasks...)
	return transferTasksToAppend, crossClusterTasksToAppend, replicationTasksToAppend, timerTasksToAppend, nil
}

func (d *nosqlExecutionStore) prepareTransferTasksForWorkflowTxn(domainID, workflowID, runID string, transferTasks []persistence.Task) ([]*nosqlplugin.TransferTask, error) {
	var tasks []*nosqlplugin.TransferTask

	for _, task := range transferTasks {
		var taskList string
		var scheduleID int64
		targetDomainID := domainID
		targetDomainIDs := map[string]struct{}{}
		targetWorkflowID := persistence.TransferTaskTransferTargetWorkflowID
		targetRunID := persistence.TransferTaskTransferTargetRunID
		targetChildWorkflowOnly := false
		recordVisibility := false

		switch task.GetType() {
		case persistence.TransferTaskTypeActivityTask:
			targetDomainID = task.(*persistence.ActivityTask).DomainID
			taskList = task.(*persistence.ActivityTask).TaskList
			scheduleID = task.(*persistence.ActivityTask).ScheduleID

		case persistence.TransferTaskTypeDecisionTask:
			targetDomainID = task.(*persistence.DecisionTask).DomainID
			taskList = task.(*persistence.DecisionTask).TaskList
			scheduleID = task.(*persistence.DecisionTask).ScheduleID
			recordVisibility = task.(*persistence.DecisionTask).RecordVisibility

		case persistence.TransferTaskTypeCancelExecution:
			targetDomainID = task.(*persistence.CancelExecutionTask).TargetDomainID
			targetWorkflowID = task.(*persistence.CancelExecutionTask).TargetWorkflowID
			targetRunID = task.(*persistence.CancelExecutionTask).TargetRunID
			if targetRunID == "" {
				targetRunID = persistence.TransferTaskTransferTargetRunID
			}
			targetChildWorkflowOnly = task.(*persistence.CancelExecutionTask).TargetChildWorkflowOnly
			scheduleID = task.(*persistence.CancelExecutionTask).InitiatedID

		case persistence.TransferTaskTypeSignalExecution:
			targetDomainID = task.(*persistence.SignalExecutionTask).TargetDomainID
			targetWorkflowID = task.(*persistence.SignalExecutionTask).TargetWorkflowID
			targetRunID = task.(*persistence.SignalExecutionTask).TargetRunID
			if targetRunID == "" {
				targetRunID = persistence.TransferTaskTransferTargetRunID
			}
			targetChildWorkflowOnly = task.(*persistence.SignalExecutionTask).TargetChildWorkflowOnly
			scheduleID = task.(*persistence.SignalExecutionTask).InitiatedID

		case persistence.TransferTaskTypeStartChildExecution:
			targetDomainID = task.(*persistence.StartChildExecutionTask).TargetDomainID
			targetWorkflowID = task.(*persistence.StartChildExecutionTask).TargetWorkflowID
			scheduleID = task.(*persistence.StartChildExecutionTask).InitiatedID

		case persistence.TransferTaskTypeRecordChildExecutionCompleted:
			targetDomainID = task.(*persistence.RecordChildExecutionCompletedTask).TargetDomainID
			targetWorkflowID = task.(*persistence.RecordChildExecutionCompletedTask).TargetWorkflowID
			targetRunID = task.(*persistence.RecordChildExecutionCompletedTask).TargetRunID
			if targetRunID == "" {
				targetRunID = persistence.TransferTaskTransferTargetRunID
			}

		case persistence.TransferTaskTypeApplyParentClosePolicy:
			targetDomainIDs = task.(*persistence.ApplyParentClosePolicyTask).TargetDomainIDs

		case persistence.TransferTaskTypeCloseExecution,
			persistence.TransferTaskTypeRecordWorkflowStarted,
			persistence.TransferTaskTypeResetWorkflow,
			persistence.TransferTaskTypeUpsertWorkflowSearchAttributes,
			persistence.TransferTaskTypeRecordWorkflowClosed:
			// No explicit property needs to be set

		default:
			return nil, &types.InternalServiceError{
				Message: fmt.Sprintf("Unknown transfer type: %v", task.GetType()),
			}
		}
		t := &nosqlplugin.TransferTask{
			TaskType:                task.GetType(),
			DomainID:                domainID,
			WorkflowID:              workflowID,
			RunID:                   runID,
			VisibilityTimestamp:     task.GetVisibilityTimestamp(),
			TaskID:                  task.GetTaskID(),
			TargetDomainID:          targetDomainID,
			TargetDomainIDs:         targetDomainIDs,
			TargetWorkflowID:        targetWorkflowID,
			TargetRunID:             targetRunID,
			TargetChildWorkflowOnly: targetChildWorkflowOnly,
			TaskList:                taskList,
			ScheduleID:              scheduleID,
			RecordVisibility:        recordVisibility,
			Version:                 task.GetVersion(),
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (d *nosqlExecutionStore) prepareActivityInfosForWorkflowTxn(activityInfos []*persistence.InternalActivityInfo) (map[int64]*persistence.InternalActivityInfo, error) {
	m := map[int64]*persistence.InternalActivityInfo{}
	for _, a := range activityInfos {
		_, scheduleEncoding := persistence.FromDataBlob(a.ScheduledEvent)
		_, startEncoding := persistence.FromDataBlob(a.StartedEvent)
		if a.StartedEvent != nil && scheduleEncoding != startEncoding {
			return nil, persistence.NewCadenceSerializationError(fmt.Sprintf("expect to have the same encoding, but %v != %v", scheduleEncoding, startEncoding))
		}
		a.ScheduledEvent = a.ScheduledEvent.ToNilSafeDataBlob()
		a.StartedEvent = a.StartedEvent.ToNilSafeDataBlob()
		m[a.ScheduleID] = a
	}
	return m, nil
}

func (d *nosqlExecutionStore) prepareTimerInfosForWorkflowTxn(timerInfo []*persistence.TimerInfo) (map[string]*persistence.TimerInfo, error) {
	m := map[string]*persistence.TimerInfo{}
	for _, a := range timerInfo {
		m[a.TimerID] = a
	}
	return m, nil
}

func (d *nosqlExecutionStore) prepareChildWFInfosForWorkflowTxn(childWFInfos []*persistence.InternalChildExecutionInfo) (map[int64]*persistence.InternalChildExecutionInfo, error) {
	m := map[int64]*persistence.InternalChildExecutionInfo{}
	for _, c := range childWFInfos {
		_, initiatedEncoding := persistence.FromDataBlob(c.InitiatedEvent)
		_, startEncoding := persistence.FromDataBlob(c.StartedEvent)
		if c.StartedEvent != nil && initiatedEncoding != startEncoding {
			return nil, persistence.NewCadenceSerializationError(fmt.Sprintf("expect to have the same encoding, but %v != %v", initiatedEncoding, startEncoding))
		}

		if c.StartedRunID == "" {
			c.StartedRunID = emptyRunID
		}

		c.InitiatedEvent = c.InitiatedEvent.ToNilSafeDataBlob()
		c.StartedEvent = c.StartedEvent.ToNilSafeDataBlob()
		m[c.InitiatedID] = c
	}
	return m, nil
}

func (d *nosqlExecutionStore) prepareRequestCancelsForWorkflowTxn(requestCancels []*persistence.RequestCancelInfo) (map[int64]*persistence.RequestCancelInfo, error) {
	m := map[int64]*persistence.RequestCancelInfo{}
	for _, c := range requestCancels {
		m[c.InitiatedID] = c
	}
	return m, nil
}

func (d *nosqlExecutionStore) prepareSignalInfosForWorkflowTxn(signalInfos []*persistence.SignalInfo) (map[int64]*persistence.SignalInfo, error) {
	m := map[int64]*persistence.SignalInfo{}
	for _, c := range signalInfos {
		m[c.InitiatedID] = c
	}
	return m, nil
}

func (d *nosqlExecutionStore) prepareUpdateWorkflowExecutionTxn(
	executionInfo *persistence.InternalWorkflowExecutionInfo,
	versionHistories *persistence.DataBlob,
	checksum checksum.Checksum,
	nowTimestamp time.Time,
	lastWriteVersion int64,
) (*nosqlplugin.WorkflowExecutionRequest, error) {
	// validate workflow state & close status
	if err := persistence.ValidateUpdateWorkflowStateCloseStatus(
		executionInfo.State,
		executionInfo.CloseStatus); err != nil {
		return nil, err
	}

	if executionInfo.ParentDomainID == "" {
		executionInfo.ParentDomainID = emptyDomainID
		executionInfo.ParentWorkflowID = ""
		executionInfo.ParentRunID = emptyRunID
		executionInfo.InitiatedID = emptyInitiatedID
	}

	// TODO: remove this logic once all workflows before 0.26.x are completed
	if executionInfo.FirstExecutionRunID == "" {
		executionInfo.FirstExecutionRunID = emptyRunID
	}

	executionInfo.CompletionEvent = executionInfo.CompletionEvent.ToNilSafeDataBlob()
	executionInfo.AutoResetPoints = executionInfo.AutoResetPoints.ToNilSafeDataBlob()
	// TODO also need to set the start / current / last write version
	versionHistories = versionHistories.ToNilSafeDataBlob()
	return &nosqlplugin.WorkflowExecutionRequest{
		InternalWorkflowExecutionInfo: *executionInfo,
		VersionHistories:              versionHistories,
		Checksums:                     &checksum,
		LastWriteVersion:              lastWriteVersion,
	}, nil
}

func (d *nosqlExecutionStore) prepareCreateWorkflowExecutionTxn(
	executionInfo *persistence.InternalWorkflowExecutionInfo,
	versionHistories *persistence.DataBlob,
	checksum checksum.Checksum,
	nowTimestamp time.Time,
	lastWriteVersion int64,
) (*nosqlplugin.WorkflowExecutionRequest, error) {
	// validate workflow state & close status
	if err := persistence.ValidateCreateWorkflowStateCloseStatus(
		executionInfo.State,
		executionInfo.CloseStatus); err != nil {
		return nil, err
	}

	if executionInfo.ParentDomainID == "" {
		executionInfo.ParentDomainID = emptyDomainID
		executionInfo.ParentWorkflowID = ""
		executionInfo.ParentRunID = emptyRunID
		executionInfo.InitiatedID = emptyInitiatedID
	}

	// TODO: remove this logic once all workflows before 0.26.x are completed
	if executionInfo.FirstExecutionRunID == "" {
		executionInfo.FirstExecutionRunID = emptyRunID
	}

	if executionInfo.StartTimestamp.IsZero() {
		executionInfo.StartTimestamp = nowTimestamp
		d.logger.Error("Workflow startTimestamp not set, fallback to now",
			tag.WorkflowDomainID(executionInfo.DomainID),
			tag.WorkflowID(executionInfo.WorkflowID),
			tag.WorkflowRunID(executionInfo.RunID),
		)
	}
	executionInfo.CompletionEvent = executionInfo.CompletionEvent.ToNilSafeDataBlob()
	executionInfo.AutoResetPoints = executionInfo.AutoResetPoints.ToNilSafeDataBlob()
	if versionHistories == nil {
		return nil, &types.InternalServiceError{Message: "encounter empty version histories in createExecution"}
	}
	versionHistories = versionHistories.ToNilSafeDataBlob()
	return &nosqlplugin.WorkflowExecutionRequest{
		InternalWorkflowExecutionInfo: *executionInfo,
		VersionHistories:              versionHistories,
		Checksums:                     &checksum,
		LastWriteVersion:              lastWriteVersion,
	}, nil
}

func (d *nosqlExecutionStore) prepareCurrentWorkflowRequestForCreateWorkflowTxn(
	domainID, workflowID, runID string,
	executionInfo *persistence.InternalWorkflowExecutionInfo,
	lastWriteVersion int64,
	request *persistence.InternalCreateWorkflowExecutionRequest,
) (*nosqlplugin.CurrentWorkflowWriteRequest, error) {
	currentWorkflowWriteReq := &nosqlplugin.CurrentWorkflowWriteRequest{
		Row: nosqlplugin.CurrentWorkflowRow{
			ShardID:          d.shardID,
			DomainID:         domainID,
			WorkflowID:       workflowID,
			RunID:            runID,
			State:            executionInfo.State,
			CloseStatus:      executionInfo.CloseStatus,
			CreateRequestID:  executionInfo.CreateRequestID,
			LastWriteVersion: lastWriteVersion,
		},
	}
	switch request.Mode {
	case persistence.CreateWorkflowModeZombie:
		// noop
		currentWorkflowWriteReq.WriteMode = nosqlplugin.CurrentWorkflowWriteModeNoop
	case persistence.CreateWorkflowModeContinueAsNew:
		currentWorkflowWriteReq.WriteMode = nosqlplugin.CurrentWorkflowWriteModeUpdate
		currentWorkflowWriteReq.Condition = &nosqlplugin.CurrentWorkflowWriteCondition{
			CurrentRunID: common.StringPtr(request.PreviousRunID),
		}
	case persistence.CreateWorkflowModeWorkflowIDReuse:
		currentWorkflowWriteReq.WriteMode = nosqlplugin.CurrentWorkflowWriteModeUpdate
		currentWorkflowWriteReq.Condition = &nosqlplugin.CurrentWorkflowWriteCondition{
			CurrentRunID:     common.StringPtr(request.PreviousRunID),
			State:            common.IntPtr(persistence.WorkflowStateCompleted),
			LastWriteVersion: common.Int64Ptr(request.PreviousLastWriteVersion),
		}
	case persistence.CreateWorkflowModeBrandNew:
		currentWorkflowWriteReq.WriteMode = nosqlplugin.CurrentWorkflowWriteModeInsert
	default:
		return nil, &types.InternalServiceError{
			Message: fmt.Sprintf("unknown mode: %v", request.Mode),
		}
	}
	return currentWorkflowWriteReq, nil
}

func (d *nosqlExecutionStore) processUpdateWorkflowResult(err error, rangeID int64) error {
	if err != nil {
		conditionFailureErr, isConditionFailedError := err.(*nosqlplugin.WorkflowOperationConditionFailure)
		if isConditionFailedError {
			switch {
			case conditionFailureErr.UnknownConditionFailureDetails != nil:
				return &persistence.ConditionFailedError{
					Msg: *conditionFailureErr.UnknownConditionFailureDetails,
				}
			case conditionFailureErr.ShardRangeIDNotMatch != nil:
				return &persistence.ShardOwnershipLostError{
					ShardID: d.shardID,
					Msg: fmt.Sprintf("Failed to update workflow execution.  Request RangeID: %v, Actual RangeID: %v",
						rangeID, *conditionFailureErr.ShardRangeIDNotMatch),
				}
			case conditionFailureErr.CurrentWorkflowConditionFailInfo != nil:
				return &persistence.CurrentWorkflowConditionFailedError{
					Msg: *conditionFailureErr.CurrentWorkflowConditionFailInfo,
				}
			default:
				// If ever runs into this branch, there is bug in the code either in here, or in the implementation of nosql plugin
				err := fmt.Errorf("unexpected conditionFailureReason error")
				d.logger.Error("A code bug exists in persistence layer, please investigate ASAP", tag.Error(err))
				return err
			}
		}
		return convertCommonErrors(d.db, "UpdateWorkflowExecution", err)
	}

	return nil
}

func (d *nosqlExecutionStore) assertNotCurrentExecution(
	ctx context.Context,
	domainID string,
	workflowID string,
	runID string,
) error {

	if resp, err := d.GetCurrentExecution(ctx, &persistence.GetCurrentExecutionRequest{
		DomainID:   domainID,
		WorkflowID: workflowID,
	}); err != nil {
		if _, ok := err.(*types.EntityNotExistsError); ok {
			// allow bypassing no current record
			return nil
		}
		return err
	} else if resp.RunID == runID {
		return &persistence.ConditionFailedError{
			Msg: fmt.Sprintf("Assertion on current record failed. Current run ID is not expected: %v", resp.RunID),
		}
	}

	return nil
}
