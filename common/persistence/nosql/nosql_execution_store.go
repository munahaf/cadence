// Copyright (c) 2017-2021 Uber Technologies, Inc.
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

	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/nosql/nosqlplugin"
	"github.com/uber/cadence/common/types"
)

// Implements ExecutionStore
type nosqlExecutionStore struct {
	shardID int
	nosqlStore
}

// NewExecutionStore is used to create an instance of ExecutionStore implementation
func NewExecutionStore(
	shardID int,
	db nosqlplugin.DB,
	logger log.Logger,
) (persistence.ExecutionStore, error) {
	return &nosqlExecutionStore{
		nosqlStore: nosqlStore{
			logger: logger,
			db:     db,
		},
		shardID: shardID,
	}, nil
}

func (d *nosqlExecutionStore) GetShardID() int {
	return d.shardID
}

func (d *nosqlExecutionStore) CreateWorkflowExecution(
	ctx context.Context,
	request *persistence.InternalCreateWorkflowExecutionRequest,
) (*persistence.CreateWorkflowExecutionResponse, error) {

	newWorkflow := request.NewWorkflowSnapshot
	executionInfo := newWorkflow.ExecutionInfo
	lastWriteVersion := newWorkflow.LastWriteVersion
	domainID := executionInfo.DomainID
	workflowID := executionInfo.WorkflowID
	runID := executionInfo.RunID

	if err := persistence.ValidateCreateWorkflowModeState(
		request.Mode,
		newWorkflow,
	); err != nil {
		return nil, err
	}

	currentWorkflowWriteReq, err := d.prepareCurrentWorkflowRequestForCreateWorkflowTxn(domainID, workflowID, runID, executionInfo, lastWriteVersion, request)
	if err != nil {
		return nil, err
	}

	workflowExecutionWriteReq, err := d.prepareCreateWorkflowExecutionRequestWithMaps(&newWorkflow)
	if err != nil {
		return nil, err
	}

	transferTasks, crossClusterTasks, replicationTasks, timerTasks, err := d.prepareNoSQLTasksForWorkflowTxn(
		domainID, workflowID, runID,
		newWorkflow.TransferTasks, newWorkflow.CrossClusterTasks, newWorkflow.ReplicationTasks, newWorkflow.TimerTasks,
		nil, nil, nil, nil,
	)
	if err != nil {
		return nil, err
	}

	shardCondition := &nosqlplugin.ShardCondition{
		ShardID: d.shardID,
		RangeID: request.RangeID,
	}

	err = d.db.InsertWorkflowExecutionWithTasks(
		ctx,
		currentWorkflowWriteReq, workflowExecutionWriteReq,
		transferTasks, crossClusterTasks, replicationTasks, timerTasks,
		shardCondition,
	)
	if err != nil {
		conditionFailureErr, isConditionFailedError := err.(*nosqlplugin.WorkflowOperationConditionFailure)
		if isConditionFailedError {
			switch {
			case conditionFailureErr.UnknownConditionFailureDetails != nil:
				return nil, &persistence.ShardOwnershipLostError{
					ShardID: d.shardID,
					Msg:     *conditionFailureErr.UnknownConditionFailureDetails,
				}
			case conditionFailureErr.ShardRangeIDNotMatch != nil:
				return nil, &persistence.ShardOwnershipLostError{
					ShardID: d.shardID,
					Msg: fmt.Sprintf("Failed to create workflow execution.  Request RangeID: %v, Actual RangeID: %v",
						request.RangeID, *conditionFailureErr.ShardRangeIDNotMatch),
				}
			case conditionFailureErr.CurrentWorkflowConditionFailInfo != nil:
				return nil, &persistence.CurrentWorkflowConditionFailedError{
					Msg: *conditionFailureErr.CurrentWorkflowConditionFailInfo,
				}
			case conditionFailureErr.WorkflowExecutionAlreadyExists != nil:
				return nil, &persistence.WorkflowExecutionAlreadyStartedError{
					Msg:              conditionFailureErr.WorkflowExecutionAlreadyExists.OtherInfo,
					StartRequestID:   conditionFailureErr.WorkflowExecutionAlreadyExists.CreateRequestID,
					RunID:            conditionFailureErr.WorkflowExecutionAlreadyExists.RunID,
					State:            conditionFailureErr.WorkflowExecutionAlreadyExists.State,
					CloseStatus:      conditionFailureErr.WorkflowExecutionAlreadyExists.CloseStatus,
					LastWriteVersion: conditionFailureErr.WorkflowExecutionAlreadyExists.LastWriteVersion,
				}
			default:
				// If ever runs into this branch, there is bug in the code either in here, or in the implementation of nosql plugin
				err := fmt.Errorf("unsupported conditionFailureReason error")
				d.logger.Error("A code bug exists in persistence layer, please investigate ASAP", tag.Error(err))
				return nil, err
			}
		}
		return nil, convertCommonErrors(d.db, "CreateWorkflowExecution", err)
	}

	return &persistence.CreateWorkflowExecutionResponse{}, nil
}

func (d *nosqlExecutionStore) GetWorkflowExecution(
	ctx context.Context,
	request *persistence.InternalGetWorkflowExecutionRequest,
) (*persistence.InternalGetWorkflowExecutionResponse, error) {

	execution := request.Execution
	state, err := d.db.SelectWorkflowExecution(ctx, d.shardID, request.DomainID, execution.WorkflowID, execution.RunID)
	if err != nil {
		if d.db.IsNotFoundError(err) {
			return nil, &types.EntityNotExistsError{
				Message: fmt.Sprintf("Workflow execution not found.  WorkflowId: %v, RunId: %v",
					execution.WorkflowID, execution.RunID),
			}
		}

		return nil, convertCommonErrors(d.db, "GetWorkflowExecution", err)
	}

	return &persistence.InternalGetWorkflowExecutionResponse{State: state}, nil
}

func (d *nosqlExecutionStore) UpdateWorkflowExecution(
	ctx context.Context,
	request *persistence.InternalUpdateWorkflowExecutionRequest,
) error {
	updateWorkflow := request.UpdateWorkflowMutation
	newWorkflow := request.NewWorkflowSnapshot

	executionInfo := updateWorkflow.ExecutionInfo
	domainID := executionInfo.DomainID
	workflowID := executionInfo.WorkflowID
	runID := executionInfo.RunID

	if err := persistence.ValidateUpdateWorkflowModeState(
		request.Mode,
		updateWorkflow,
		newWorkflow,
	); err != nil {
		return err
	}

	var currentWorkflowWriteReq *nosqlplugin.CurrentWorkflowWriteRequest

	switch request.Mode {
	case persistence.UpdateWorkflowModeIgnoreCurrent:
		currentWorkflowWriteReq = &nosqlplugin.CurrentWorkflowWriteRequest{
			WriteMode: nosqlplugin.CurrentWorkflowWriteModeNoop,
		}
	case persistence.UpdateWorkflowModeBypassCurrent:
		if err := d.assertNotCurrentExecution(
			ctx,
			domainID,
			workflowID,
			runID); err != nil {
			return err
		}
		currentWorkflowWriteReq = &nosqlplugin.CurrentWorkflowWriteRequest{
			WriteMode: nosqlplugin.CurrentWorkflowWriteModeNoop,
		}

	case persistence.UpdateWorkflowModeUpdateCurrent:
		if newWorkflow != nil {
			newExecutionInfo := newWorkflow.ExecutionInfo
			newLastWriteVersion := newWorkflow.LastWriteVersion
			newDomainID := newExecutionInfo.DomainID
			// TODO: ?? would it change at all ??
			newWorkflowID := newExecutionInfo.WorkflowID
			newRunID := newExecutionInfo.RunID

			if domainID != newDomainID {
				return &types.InternalServiceError{
					Message: "UpdateWorkflowExecution: cannot continue as new to another domain",
				}
			}

			currentWorkflowWriteReq = &nosqlplugin.CurrentWorkflowWriteRequest{
				WriteMode: nosqlplugin.CurrentWorkflowWriteModeUpdate,
				Row: nosqlplugin.CurrentWorkflowRow{
					ShardID:          d.shardID,
					DomainID:         newDomainID,
					WorkflowID:       newWorkflowID,
					RunID:            newRunID,
					State:            newExecutionInfo.State,
					CloseStatus:      newExecutionInfo.CloseStatus,
					CreateRequestID:  newExecutionInfo.CreateRequestID,
					LastWriteVersion: newLastWriteVersion,
				},
				Condition: &nosqlplugin.CurrentWorkflowWriteCondition{
					CurrentRunID: &runID,
				},
			}
		} else {
			lastWriteVersion := updateWorkflow.LastWriteVersion

			currentWorkflowWriteReq = &nosqlplugin.CurrentWorkflowWriteRequest{
				WriteMode: nosqlplugin.CurrentWorkflowWriteModeUpdate,
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
				Condition: &nosqlplugin.CurrentWorkflowWriteCondition{
					CurrentRunID: &runID,
				},
			}
		}

	default:
		return &types.InternalServiceError{
			Message: fmt.Sprintf("UpdateWorkflowExecution: unknown mode: %v", request.Mode),
		}
	}

	var mutateExecution, insertExecution *nosqlplugin.WorkflowExecutionRequest
	var nosqlTransferTasks []*nosqlplugin.TransferTask
	var nosqlCrossClusterTasks []*nosqlplugin.CrossClusterTask
	var nosqlReplicationTasks []*nosqlplugin.ReplicationTask
	var nosqlTimerTasks []*nosqlplugin.TimerTask
	var err error

	// 1. current
	mutateExecution, err = d.prepareUpdateWorkflowExecutionRequestWithMapsAndEventBuffer(&updateWorkflow)
	if err != nil {
		return err
	}
	nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks, err = d.prepareNoSQLTasksForWorkflowTxn(
		domainID, workflowID, updateWorkflow.ExecutionInfo.RunID,
		updateWorkflow.TransferTasks, updateWorkflow.CrossClusterTasks, updateWorkflow.ReplicationTasks, updateWorkflow.TimerTasks,
		nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks,
	)
	if err != nil {
		return err
	}

	// 2. new
	if newWorkflow != nil {
		insertExecution, err = d.prepareCreateWorkflowExecutionRequestWithMaps(newWorkflow)
		if err != nil {
			return err
		}

		nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks, err = d.prepareNoSQLTasksForWorkflowTxn(
			domainID, workflowID, newWorkflow.ExecutionInfo.RunID,
			newWorkflow.TransferTasks, newWorkflow.CrossClusterTasks, newWorkflow.ReplicationTasks, newWorkflow.TimerTasks,
			nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks,
		)
		if err != nil {
			return err
		}
	}

	shardCondition := &nosqlplugin.ShardCondition{
		ShardID: d.shardID,
		RangeID: request.RangeID,
	}

	err = d.db.UpdateWorkflowExecutionWithTasks(
		ctx, currentWorkflowWriteReq,
		mutateExecution, insertExecution, nil, // no workflow to reset here
		nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks,
		shardCondition)

	return d.processUpdateWorkflowResult(err, request.RangeID)
}

func (d *nosqlExecutionStore) ConflictResolveWorkflowExecution(
	ctx context.Context,
	request *persistence.InternalConflictResolveWorkflowExecutionRequest,
) error {
	currentWorkflow := request.CurrentWorkflowMutation
	resetWorkflow := request.ResetWorkflowSnapshot
	newWorkflow := request.NewWorkflowSnapshot

	domainID := resetWorkflow.ExecutionInfo.DomainID
	workflowID := resetWorkflow.ExecutionInfo.WorkflowID

	if err := persistence.ValidateConflictResolveWorkflowModeState(
		request.Mode,
		resetWorkflow,
		newWorkflow,
		currentWorkflow,
	); err != nil {
		return err
	}

	var currentWorkflowWriteReq *nosqlplugin.CurrentWorkflowWriteRequest
	var prevRunID string

	switch request.Mode {
	case persistence.ConflictResolveWorkflowModeBypassCurrent:
		if err := d.assertNotCurrentExecution(
			ctx,
			domainID,
			workflowID,
			resetWorkflow.ExecutionInfo.RunID); err != nil {
			return err
		}
		currentWorkflowWriteReq = &nosqlplugin.CurrentWorkflowWriteRequest{
			WriteMode: nosqlplugin.CurrentWorkflowWriteModeNoop,
		}
	case persistence.ConflictResolveWorkflowModeUpdateCurrent:
		executionInfo := resetWorkflow.ExecutionInfo
		lastWriteVersion := resetWorkflow.LastWriteVersion
		if newWorkflow != nil {
			executionInfo = newWorkflow.ExecutionInfo
			lastWriteVersion = newWorkflow.LastWriteVersion
		}

		if currentWorkflow != nil {
			prevRunID = currentWorkflow.ExecutionInfo.RunID
		} else {
			// reset workflow is current
			prevRunID = resetWorkflow.ExecutionInfo.RunID
		}
		currentWorkflowWriteReq = &nosqlplugin.CurrentWorkflowWriteRequest{
			WriteMode: nosqlplugin.CurrentWorkflowWriteModeUpdate,
			Row: nosqlplugin.CurrentWorkflowRow{
				ShardID:          d.shardID,
				DomainID:         domainID,
				WorkflowID:       workflowID,
				RunID:            executionInfo.RunID,
				State:            executionInfo.State,
				CloseStatus:      executionInfo.CloseStatus,
				CreateRequestID:  executionInfo.CreateRequestID,
				LastWriteVersion: lastWriteVersion,
			},
			Condition: &nosqlplugin.CurrentWorkflowWriteCondition{
				CurrentRunID: &prevRunID,
			},
		}

	default:
		return &types.InternalServiceError{
			Message: fmt.Sprintf("ConflictResolveWorkflowExecution: unknown mode: %v", request.Mode),
		}
	}

	var mutateExecution, insertExecution, resetExecution *nosqlplugin.WorkflowExecutionRequest
	var nosqlTransferTasks []*nosqlplugin.TransferTask
	var nosqlCrossClusterTasks []*nosqlplugin.CrossClusterTask
	var nosqlReplicationTasks []*nosqlplugin.ReplicationTask
	var nosqlTimerTasks []*nosqlplugin.TimerTask
	var err error

	// 1. current
	if currentWorkflow != nil {
		mutateExecution, err = d.prepareUpdateWorkflowExecutionRequestWithMapsAndEventBuffer(currentWorkflow)
		if err != nil {
			return err
		}
		nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks, err = d.prepareNoSQLTasksForWorkflowTxn(
			domainID, workflowID, currentWorkflow.ExecutionInfo.RunID,
			currentWorkflow.TransferTasks, currentWorkflow.CrossClusterTasks, currentWorkflow.ReplicationTasks, currentWorkflow.TimerTasks,
			nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks,
		)
		if err != nil {
			return err
		}
	}

	// 2. reset
	resetExecution, err = d.prepareResetWorkflowExecutionRequestWithMapsAndEventBuffer(&resetWorkflow)
	if err != nil {
		return err
	}
	nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks, err = d.prepareNoSQLTasksForWorkflowTxn(
		domainID, workflowID, resetWorkflow.ExecutionInfo.RunID,
		resetWorkflow.TransferTasks, resetWorkflow.CrossClusterTasks, resetWorkflow.ReplicationTasks, resetWorkflow.TimerTasks,
		nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks,
	)
	if err != nil {
		return err
	}

	// 3. new
	if newWorkflow != nil {
		insertExecution, err = d.prepareCreateWorkflowExecutionRequestWithMaps(newWorkflow)
		if err != nil {
			return err
		}

		nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks, err = d.prepareNoSQLTasksForWorkflowTxn(
			domainID, workflowID, newWorkflow.ExecutionInfo.RunID,
			newWorkflow.TransferTasks, newWorkflow.CrossClusterTasks, newWorkflow.ReplicationTasks, newWorkflow.TimerTasks,
			nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks,
		)
		if err != nil {
			return err
		}
	}

	shardCondition := &nosqlplugin.ShardCondition{
		ShardID: d.shardID,
		RangeID: request.RangeID,
	}

	err = d.db.UpdateWorkflowExecutionWithTasks(
		ctx, currentWorkflowWriteReq,
		mutateExecution, insertExecution, resetExecution,
		nosqlTransferTasks, nosqlCrossClusterTasks, nosqlReplicationTasks, nosqlTimerTasks,
		shardCondition)
	return d.processUpdateWorkflowResult(err, request.RangeID)
}

func (d *nosqlExecutionStore) DeleteWorkflowExecution(
	ctx context.Context,
	request *persistence.DeleteWorkflowExecutionRequest,
) error {
	err := d.db.DeleteWorkflowExecution(ctx, d.shardID, request.DomainID, request.WorkflowID, request.RunID)
	if err != nil {
		return convertCommonErrors(d.db, "DeleteWorkflowExecution", err)
	}

	return nil
}

func (d *nosqlExecutionStore) DeleteCurrentWorkflowExecution(
	ctx context.Context,
	request *persistence.DeleteCurrentWorkflowExecutionRequest,
) error {
	err := d.db.DeleteCurrentWorkflow(ctx, d.shardID, request.DomainID, request.WorkflowID, request.RunID)
	if err != nil {
		return convertCommonErrors(d.db, "DeleteCurrentWorkflowExecution", err)
	}

	return nil
}

func (d *nosqlExecutionStore) GetCurrentExecution(
	ctx context.Context,
	request *persistence.GetCurrentExecutionRequest,
) (*persistence.GetCurrentExecutionResponse,
	error) {
	result, err := d.db.SelectCurrentWorkflow(ctx, d.shardID, request.DomainID, request.WorkflowID)

	if err != nil {
		if d.db.IsNotFoundError(err) {
			return nil, &types.EntityNotExistsError{
				Message: fmt.Sprintf("Workflow execution not found.  WorkflowId: %v",
					request.WorkflowID),
			}
		}
		return nil, convertCommonErrors(d.db, "GetCurrentExecution", err)
	}

	return &persistence.GetCurrentExecutionResponse{
		RunID:            result.RunID,
		StartRequestID:   result.CreateRequestID,
		State:            result.State,
		CloseStatus:      result.CloseStatus,
		LastWriteVersion: result.LastWriteVersion,
	}, nil
}

func (d *nosqlExecutionStore) ListCurrentExecutions(
	ctx context.Context,
	request *persistence.ListCurrentExecutionsRequest,
) (*persistence.ListCurrentExecutionsResponse, error) {
	executions, token, err := d.db.SelectAllCurrentWorkflows(ctx, d.shardID, request.PageToken, request.PageSize)
	if err != nil {
		return nil, convertCommonErrors(d.db, "ListCurrentExecutions", err)
	}
	return &persistence.ListCurrentExecutionsResponse{
		Executions: executions,
		PageToken:  token,
	}, nil
}

func (d *nosqlExecutionStore) IsWorkflowExecutionExists(
	ctx context.Context,
	request *persistence.IsWorkflowExecutionExistsRequest,
) (*persistence.IsWorkflowExecutionExistsResponse, error) {
	exists, err := d.db.IsWorkflowExecutionExists(ctx, d.shardID, request.DomainID, request.WorkflowID, request.RunID)
	if err != nil {
		return nil, convertCommonErrors(d.db, "IsWorkflowExecutionExists", err)
	}
	return &persistence.IsWorkflowExecutionExistsResponse{
		Exists: exists,
	}, nil
}

func (d *nosqlExecutionStore) ListConcreteExecutions(
	ctx context.Context,
	request *persistence.ListConcreteExecutionsRequest,
) (*persistence.InternalListConcreteExecutionsResponse, error) {
	executions, nextPageToken, err := d.db.SelectAllWorkflowExecutions(ctx, d.shardID, request.PageToken, request.PageSize)
	if err != nil {
		return nil, convertCommonErrors(d.db, "ListConcreteExecutions", err)
	}
	return &persistence.InternalListConcreteExecutionsResponse{
		Executions:    executions,
		NextPageToken: nextPageToken,
	}, nil
}

func (d *nosqlExecutionStore) GetTransferTasks(
	ctx context.Context,
	request *persistence.GetTransferTasksRequest,
) (*persistence.GetTransferTasksResponse, error) {

	tasks, nextPageToken, err := d.db.SelectTransferTasksOrderByTaskID(ctx, d.shardID, request.BatchSize, request.NextPageToken, request.ReadLevel, request.MaxReadLevel)
	if err != nil {
		return nil, convertCommonErrors(d.db, "GetTransferTasks", err)
	}

	return &persistence.GetTransferTasksResponse{
		Tasks:         tasks,
		NextPageToken: nextPageToken,
	}, nil
}

func (d *nosqlExecutionStore) GetCrossClusterTasks(
	ctx context.Context,
	request *persistence.GetCrossClusterTasksRequest,
) (*persistence.GetCrossClusterTasksResponse, error) {

	cTasks, nextPageToken, err := d.db.SelectCrossClusterTasksOrderByTaskID(ctx, d.shardID, request.BatchSize, request.NextPageToken, request.TargetCluster, request.ReadLevel, request.MaxReadLevel)

	if err != nil {
		return nil, convertCommonErrors(d.db, "GetCrossClusterTasks", err)
	}

	var tTasks []*persistence.CrossClusterTaskInfo
	for _, t := range cTasks {
		// revive:disable-next-line:range-val-address Appending address of TransferTask, not of t.
		tTasks = append(tTasks, &t.TransferTask)
	}
	return &persistence.GetCrossClusterTasksResponse{
		Tasks:         tTasks,
		NextPageToken: nextPageToken,
	}, nil
}

func (d *nosqlExecutionStore) GetReplicationTasks(
	ctx context.Context,
	request *persistence.GetReplicationTasksRequest,
) (*persistence.InternalGetReplicationTasksResponse, error) {

	tasks, nextPageToken, err := d.db.SelectReplicationTasksOrderByTaskID(ctx, d.shardID, request.BatchSize, request.NextPageToken, request.ReadLevel, request.MaxReadLevel)
	if err != nil {
		return nil, convertCommonErrors(d.db, "GetReplicationTasks", err)
	}
	return &persistence.InternalGetReplicationTasksResponse{
		Tasks:         tasks,
		NextPageToken: nextPageToken,
	}, nil
}

func (d *nosqlExecutionStore) CompleteTransferTask(
	ctx context.Context,
	request *persistence.CompleteTransferTaskRequest,
) error {
	err := d.db.DeleteTransferTask(ctx, d.shardID, request.TaskID)
	if err != nil {
		return convertCommonErrors(d.db, "CompleteTransferTask", err)
	}

	return nil
}

func (d *nosqlExecutionStore) RangeCompleteTransferTask(
	ctx context.Context,
	request *persistence.RangeCompleteTransferTaskRequest,
) (*persistence.RangeCompleteTransferTaskResponse, error) {
	err := d.db.RangeDeleteTransferTasks(ctx, d.shardID, request.ExclusiveBeginTaskID, request.InclusiveEndTaskID)
	if err != nil {
		return nil, convertCommonErrors(d.db, "RangeCompleteTransferTask", err)
	}

	return &persistence.RangeCompleteTransferTaskResponse{TasksCompleted: persistence.UnknownNumRowsAffected}, nil
}

func (d *nosqlExecutionStore) CompleteCrossClusterTask(
	ctx context.Context,
	request *persistence.CompleteCrossClusterTaskRequest,
) error {

	err := d.db.DeleteCrossClusterTask(ctx, d.shardID, request.TargetCluster, request.TaskID)
	if err != nil {
		return convertCommonErrors(d.db, "CompleteCrossClusterTask", err)
	}

	return nil
}

func (d *nosqlExecutionStore) RangeCompleteCrossClusterTask(
	ctx context.Context,
	request *persistence.RangeCompleteCrossClusterTaskRequest,
) (*persistence.RangeCompleteCrossClusterTaskResponse, error) {

	err := d.db.RangeDeleteCrossClusterTasks(ctx, d.shardID, request.TargetCluster, request.ExclusiveBeginTaskID, request.InclusiveEndTaskID)
	if err != nil {
		return nil, convertCommonErrors(d.db, "RangeCompleteCrossClusterTask", err)
	}

	return &persistence.RangeCompleteCrossClusterTaskResponse{TasksCompleted: persistence.UnknownNumRowsAffected}, nil
}

func (d *nosqlExecutionStore) CompleteReplicationTask(
	ctx context.Context,
	request *persistence.CompleteReplicationTaskRequest,
) error {
	err := d.db.DeleteReplicationTask(ctx, d.shardID, request.TaskID)
	if err != nil {
		return convertCommonErrors(d.db, "CompleteReplicationTask", err)
	}

	return nil
}

func (d *nosqlExecutionStore) RangeCompleteReplicationTask(
	ctx context.Context,
	request *persistence.RangeCompleteReplicationTaskRequest,
) (*persistence.RangeCompleteReplicationTaskResponse, error) {

	err := d.db.RangeDeleteReplicationTasks(ctx, d.shardID, request.InclusiveEndTaskID)
	if err != nil {
		return nil, convertCommonErrors(d.db, "RangeCompleteReplicationTask", err)
	}

	return &persistence.RangeCompleteReplicationTaskResponse{TasksCompleted: persistence.UnknownNumRowsAffected}, nil
}

func (d *nosqlExecutionStore) CompleteTimerTask(
	ctx context.Context,
	request *persistence.CompleteTimerTaskRequest,
) error {
	err := d.db.DeleteTimerTask(ctx, d.shardID, request.TaskID, request.VisibilityTimestamp)
	if err != nil {
		return convertCommonErrors(d.db, "CompleteTimerTask", err)
	}

	return nil
}

func (d *nosqlExecutionStore) RangeCompleteTimerTask(
	ctx context.Context,
	request *persistence.RangeCompleteTimerTaskRequest,
) (*persistence.RangeCompleteTimerTaskResponse, error) {
	err := d.db.RangeDeleteTimerTasks(ctx, d.shardID, request.InclusiveBeginTimestamp, request.ExclusiveEndTimestamp)
	if err != nil {
		return nil, convertCommonErrors(d.db, "RangeCompleteTimerTask", err)
	}

	return &persistence.RangeCompleteTimerTaskResponse{TasksCompleted: persistence.UnknownNumRowsAffected}, nil
}

func (d *nosqlExecutionStore) GetTimerIndexTasks(
	ctx context.Context,
	request *persistence.GetTimerIndexTasksRequest,
) (*persistence.GetTimerIndexTasksResponse, error) {

	timers, nextPageToken, err := d.db.SelectTimerTasksOrderByVisibilityTime(ctx, d.shardID, request.BatchSize, request.NextPageToken, request.MinTimestamp, request.MaxTimestamp)
	if err != nil {
		return nil, convertCommonErrors(d.db, "GetTimerTasks", err)
	}

	return &persistence.GetTimerIndexTasksResponse{
		Timers:        timers,
		NextPageToken: nextPageToken,
	}, nil
}

func (d *nosqlExecutionStore) PutReplicationTaskToDLQ(
	ctx context.Context,
	request *persistence.InternalPutReplicationTaskToDLQRequest,
) error {

	err := d.db.InsertReplicationDLQTask(ctx, d.shardID, request.SourceClusterName, *request.TaskInfo)
	if err != nil {
		return convertCommonErrors(d.db, "PutReplicationTaskToDLQ", err)
	}

	return nil
}

func (d *nosqlExecutionStore) GetReplicationTasksFromDLQ(
	ctx context.Context,
	request *persistence.GetReplicationTasksFromDLQRequest,
) (*persistence.InternalGetReplicationTasksFromDLQResponse, error) {
	tasks, nextPageToken, err := d.db.SelectReplicationDLQTasksOrderByTaskID(ctx, d.shardID, request.SourceClusterName, request.BatchSize, request.NextPageToken, request.ReadLevel, request.MaxReadLevel)
	if err != nil {
		return nil, convertCommonErrors(d.db, "GetReplicationTasksFromDLQ", err)
	}
	return &persistence.InternalGetReplicationTasksResponse{
		Tasks:         tasks,
		NextPageToken: nextPageToken,
	}, nil
}

func (d *nosqlExecutionStore) GetReplicationDLQSize(
	ctx context.Context,
	request *persistence.GetReplicationDLQSizeRequest,
) (*persistence.GetReplicationDLQSizeResponse, error) {

	size, err := d.db.SelectReplicationDLQTasksCount(ctx, d.shardID, request.SourceClusterName)
	if err != nil {
		return nil, convertCommonErrors(d.db, "GetReplicationDLQSize", err)
	}
	return &persistence.GetReplicationDLQSizeResponse{
		Size: size,
	}, nil
}

func (d *nosqlExecutionStore) DeleteReplicationTaskFromDLQ(
	ctx context.Context,
	request *persistence.DeleteReplicationTaskFromDLQRequest,
) error {

	err := d.db.DeleteReplicationDLQTask(ctx, d.shardID, request.SourceClusterName, request.TaskID)
	if err != nil {
		return convertCommonErrors(d.db, "DeleteReplicationTaskFromDLQ", err)
	}

	return nil
}

func (d *nosqlExecutionStore) RangeDeleteReplicationTaskFromDLQ(
	ctx context.Context,
	request *persistence.RangeDeleteReplicationTaskFromDLQRequest,
) (*persistence.RangeDeleteReplicationTaskFromDLQResponse, error) {

	err := d.db.RangeDeleteReplicationDLQTasks(ctx, d.shardID, request.SourceClusterName, request.ExclusiveBeginTaskID, request.InclusiveEndTaskID)
	if err != nil {
		return nil, convertCommonErrors(d.db, "RangeDeleteReplicationTaskFromDLQ", err)
	}

	return &persistence.RangeDeleteReplicationTaskFromDLQResponse{TasksCompleted: persistence.UnknownNumRowsAffected}, nil
}

func (d *nosqlExecutionStore) CreateFailoverMarkerTasks(
	ctx context.Context,
	request *persistence.CreateFailoverMarkersRequest,
) error {

	var nosqlTasks []*nosqlplugin.ReplicationTask
	for _, task := range request.Markers {
		ts := []persistence.Task{task}

		tasks, err := d.prepareReplicationTasksForWorkflowTxn(task.DomainID, rowTypeReplicationWorkflowID, rowTypeReplicationRunID, ts)
		if err != nil {
			return err
		}
		nosqlTasks = append(nosqlTasks, tasks...)
	}

	err := d.db.InsertReplicationTask(ctx, nosqlTasks, nosqlplugin.ShardCondition{
		ShardID: d.shardID,
		RangeID: request.RangeID,
	})

	if err != nil {
		conditionFailureErr, isConditionFailedError := err.(*nosqlplugin.ShardOperationConditionFailure)
		if isConditionFailedError {
			return &persistence.ShardOwnershipLostError{
				ShardID: d.shardID,
				Msg: fmt.Sprintf("Failed to create workflow execution.  Request RangeID: %v, columns: (%v)",
					conditionFailureErr.RangeID, conditionFailureErr.Details),
			}
		}
	}
	return nil
}
