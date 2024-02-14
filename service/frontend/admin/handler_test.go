// Copyright (c) 2017 Uber Technologies, Inc.
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

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/uber-go/tally"

	"github.com/uber/cadence/client/frontend"
	"github.com/uber/cadence/client/history"
	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/asyncworkflow/queueconfigapi"
	"github.com/uber/cadence/common/cache"
	"github.com/uber/cadence/common/config"
	"github.com/uber/cadence/common/domain"
	"github.com/uber/cadence/common/dynamicconfig"
	esmock "github.com/uber/cadence/common/elasticsearch/mocks"
	"github.com/uber/cadence/common/isolationgroup/isolationgroupapi"
	"github.com/uber/cadence/common/log/testlogger"
	"github.com/uber/cadence/common/membership"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/mocks"
	"github.com/uber/cadence/common/partition"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/resource"
	"github.com/uber/cadence/common/service"
	"github.com/uber/cadence/common/types"
	frontendcfg "github.com/uber/cadence/service/frontend/config"
	"github.com/uber/cadence/service/frontend/validate"
)

type (
	adminHandlerSuite struct {
		suite.Suite
		*require.Assertions

		controller        *gomock.Controller
		mockResource      *resource.Test
		mockHistoryClient *history.MockClient
		mockDomainCache   *cache.MockDomainCache
		frontendClient    *frontend.MockClient
		mockResolver      *membership.MockResolver

		mockHistoryV2Mgr *mocks.HistoryV2Manager

		domainName string
		domainID   string

		handler *adminHandlerImpl
	}
)

func TestAdminHandlerSuite(t *testing.T) {
	s := new(adminHandlerSuite)
	suite.Run(t, s)
}

func (s *adminHandlerSuite) SetupTest() {
	s.Assertions = require.New(s.T())

	s.domainName = "some random domain name"
	s.domainID = "some random domain ID"

	s.controller = gomock.NewController(s.T())
	s.mockResource = resource.NewTest(s.T(), s.controller, metrics.Frontend)
	s.mockDomainCache = s.mockResource.DomainCache
	s.mockHistoryClient = s.mockResource.HistoryClient
	s.mockHistoryV2Mgr = s.mockResource.HistoryMgr
	s.frontendClient = s.mockResource.FrontendClient
	s.mockResolver = s.mockResource.MembershipResolver

	params := &resource.Params{
		Logger:          testlogger.New(s.T()),
		ThrottledLogger: testlogger.New(s.T()),
		MetricScope:     tally.NewTestScope(service.Frontend, make(map[string]string)),
		MetricsClient:   metrics.NewNoopMetricsClient(),
		PersistenceConfig: config.Persistence{
			NumHistoryShards: 1,
		},
	}
	config := &frontendcfg.Config{
		EnableAdminProtection:  dynamicconfig.GetBoolPropertyFn(false),
		EnableGracefulFailover: dynamicconfig.GetBoolPropertyFn(false),
	}

	dh := domain.NewMockHandler(s.controller)
	s.handler = NewHandler(s.mockResource, params, config, dh).(*adminHandlerImpl)
	s.handler.Start()
}

func (s *adminHandlerSuite) TearDownTest() {
	s.controller.Finish()
	s.mockResource.Finish(s.T())
	s.handler.Stop()
}

func (s *adminHandlerSuite) TestMaintainCorruptWorkflow_NormalWorkflow() {
	s.testMaintainCorruptWorkflow(nil, nil, false)
}

func (s *adminHandlerSuite) TestMaintainCorruptWorkflow_WorkflowDoesNotExist() {
	err := &types.EntityNotExistsError{Message: "Workflow does not exist"}
	s.testMaintainCorruptWorkflow(err, nil, false)
}

func (s *adminHandlerSuite) TestMaintainCorruptWorkflow_NoStartEvent() {
	s.mockDomainCache.EXPECT().GetDomainName(gomock.Any()).Return(s.domainName, nil).AnyTimes()
	err := &types.InternalServiceError{Message: "unable to get workflow start event"}
	s.testMaintainCorruptWorkflow(err, nil, true)
}
func (s *adminHandlerSuite) TestMaintainCorruptWorkflow_NoStartEventHistory() {
	s.mockDomainCache.EXPECT().GetDomainName(gomock.Any()).Return(s.domainName, nil).AnyTimes()
	err := &types.InternalServiceError{Message: "unable to get workflow start event"}
	s.testMaintainCorruptWorkflow(nil, err, true)
}

func (s *adminHandlerSuite) TestMaintainCorruptWorkflow_UnableToGetScheduledEvent() {
	s.mockDomainCache.EXPECT().GetDomainName(gomock.Any()).Return(s.domainName, nil).AnyTimes()
	err := &types.InternalServiceError{Message: "unable to get activity scheduled event"}
	s.testMaintainCorruptWorkflow(err, nil, true)
}
func (s *adminHandlerSuite) TestMaintainCorruptWorkflow_UnableToGetScheduledEventHistory() {
	s.mockDomainCache.EXPECT().GetDomainName(gomock.Any()).Return(s.domainName, nil).AnyTimes()
	err := &types.InternalServiceError{Message: "unable to get activity scheduled event"}
	s.testMaintainCorruptWorkflow(nil, err, true)
}

func (s *adminHandlerSuite) TestMaintainCorruptWorkflow_CorruptedHistory() {
	s.mockDomainCache.EXPECT().GetDomainName(gomock.Any()).Return(s.domainName, nil).AnyTimes()
	err := &types.InternalDataInconsistencyError{
		Message: "corrupted history event batch, eventID is not continouous",
	}
	s.testMaintainCorruptWorkflow(err, nil, true)
}

func (s *adminHandlerSuite) testMaintainCorruptWorkflow(
	describeWorkflowError error,
	getHistoryError error,
	expectDeletion bool,
) {
	handler := s.handler
	handler.params = &resource.Params{}
	ctx := context.Background()

	request := &types.AdminMaintainWorkflowRequest{
		Domain: s.domainName,
		Execution: &types.WorkflowExecution{
			WorkflowID: "someWorkflowID",
			RunID:      uuid.New(),
		},
		SkipErrors: true,
	}

	// need to reeturn error here to start deleting
	describeResp := &types.DescribeWorkflowExecutionResponse{}
	s.frontendClient.EXPECT().DescribeWorkflowExecution(gomock.Any(), gomock.Any()).
		Return(describeResp, describeWorkflowError)

	// need to reeturn error here to start deleting
	historyResponse := &types.GetWorkflowExecutionHistoryResponse{}
	s.frontendClient.EXPECT().GetWorkflowExecutionHistory(gomock.Any(), gomock.Any()).
		Return(historyResponse, getHistoryError).AnyTimes()

	if expectDeletion {
		hostInfo := membership.NewHostInfo("taskListA:thriftPort")
		s.mockResolver.EXPECT().Lookup(gomock.Any(), gomock.Any()).Return(hostInfo, nil)
		s.mockDomainCache.EXPECT().GetDomainID(s.domainName).Return(s.domainID, nil)

		testMutableState := &types.DescribeMutableStateResponse{
			MutableStateInDatabase: "{\"ExecutionInfo\":{\"BranchToken\":\"WQsACgAAACQ2MzI5YzEzMi1mMGI0LTQwZmUtYWYxMS1hODVmMDA3MzAzODQLABQAAAAkOWM5OWI1MjItMGEyZi00NTdmLWEyNDgtMWU0OTA0ZDg4YzVhDwAeDAAAAAAA\"}}",
		}
		s.mockHistoryClient.EXPECT().DescribeMutableState(gomock.Any(), gomock.Any()).Return(testMutableState, nil)

		s.mockHistoryV2Mgr.On("DeleteHistoryBranch", mock.Anything, mock.Anything).Return(nil).Once()
		s.mockResource.ExecutionMgr.On("DeleteWorkflowExecution", mock.Anything, mock.Anything).Return(nil).Once()
		s.mockResource.ExecutionMgr.On("DeleteCurrentWorkflowExecution", mock.Anything, mock.Anything).Return(nil).Once()
		s.mockResource.VisibilityMgr.On("DeleteWorkflowExecution", mock.Anything, mock.Anything).Return(nil).Once()
	}

	_, err := handler.MaintainCorruptWorkflow(ctx, request)
	s.Nil(err)
}

func (s *adminHandlerSuite) Test_ConvertIndexedValueTypeToESDataType() {
	tests := []struct {
		input    types.IndexedValueType
		expected string
	}{
		{
			input:    types.IndexedValueTypeString,
			expected: "text",
		},
		{
			input:    types.IndexedValueTypeKeyword,
			expected: "keyword",
		},
		{
			input:    types.IndexedValueTypeInt,
			expected: "long",
		},
		{
			input:    types.IndexedValueTypeDouble,
			expected: "double",
		},
		{
			input:    types.IndexedValueTypeBool,
			expected: "boolean",
		},
		{
			input:    types.IndexedValueTypeDatetime,
			expected: "date",
		},
		{
			input:    types.IndexedValueType(-1),
			expected: "",
		},
	}

	for _, test := range tests {
		s.Equal(test.expected, convertIndexedValueTypeToESDataType(test.input))
	}
}

func (s *adminHandlerSuite) Test_GetWorkflowExecutionRawHistoryV2_FailedOnInvalidWorkflowID() {

	ctx := context.Background()
	_, err := s.handler.GetWorkflowExecutionRawHistoryV2(ctx,
		&types.GetWorkflowExecutionRawHistoryV2Request{
			Domain: s.domainName,
			Execution: &types.WorkflowExecution{
				WorkflowID: "",
				RunID:      uuid.New(),
			},
			StartEventID:      common.Int64Ptr(1),
			StartEventVersion: common.Int64Ptr(100),
			EndEventID:        common.Int64Ptr(10),
			EndEventVersion:   common.Int64Ptr(100),
			MaximumPageSize:   1,
			NextPageToken:     nil,
		})
	s.Error(err)
}

func (s *adminHandlerSuite) Test_GetWorkflowExecutionRawHistoryV2_FailedOnInvalidRunID() {
	ctx := context.Background()
	_, err := s.handler.GetWorkflowExecutionRawHistoryV2(ctx,
		&types.GetWorkflowExecutionRawHistoryV2Request{
			Domain: s.domainName,
			Execution: &types.WorkflowExecution{
				WorkflowID: "workflowID",
				RunID:      "runID",
			},
			StartEventID:      common.Int64Ptr(1),
			StartEventVersion: common.Int64Ptr(100),
			EndEventID:        common.Int64Ptr(10),
			EndEventVersion:   common.Int64Ptr(100),
			MaximumPageSize:   1,
			NextPageToken:     nil,
		})
	s.Error(err)
}

func (s *adminHandlerSuite) Test_GetWorkflowExecutionRawHistoryV2_FailedOnInvalidSize() {
	ctx := context.Background()
	_, err := s.handler.GetWorkflowExecutionRawHistoryV2(ctx,
		&types.GetWorkflowExecutionRawHistoryV2Request{
			Domain: s.domainName,
			Execution: &types.WorkflowExecution{
				WorkflowID: "workflowID",
				RunID:      uuid.New(),
			},
			StartEventID:      common.Int64Ptr(1),
			StartEventVersion: common.Int64Ptr(100),
			EndEventID:        common.Int64Ptr(10),
			EndEventVersion:   common.Int64Ptr(100),
			MaximumPageSize:   -1,
			NextPageToken:     nil,
		})
	s.Error(err)
}

func (s *adminHandlerSuite) Test_GetWorkflowExecutionRawHistoryV2_FailedOnDomainCache() {
	ctx := context.Background()
	s.mockDomainCache.EXPECT().GetDomainID(s.domainName).Return("", fmt.Errorf("test")).Times(1)
	_, err := s.handler.GetWorkflowExecutionRawHistoryV2(ctx,
		&types.GetWorkflowExecutionRawHistoryV2Request{
			Domain: s.domainName,
			Execution: &types.WorkflowExecution{
				WorkflowID: "workflowID",
				RunID:      uuid.New(),
			},
			StartEventID:      common.Int64Ptr(1),
			StartEventVersion: common.Int64Ptr(100),
			EndEventID:        common.Int64Ptr(10),
			EndEventVersion:   common.Int64Ptr(100),
			MaximumPageSize:   1,
			NextPageToken:     nil,
		})
	s.Error(err)
}

func (s *adminHandlerSuite) Test_GetWorkflowExecutionRawHistoryV2() {
	ctx := context.Background()
	s.mockDomainCache.EXPECT().GetDomainID(s.domainName).Return(s.domainID, nil).AnyTimes()
	branchToken := []byte{1}
	versionHistory := persistence.NewVersionHistory(branchToken, []*persistence.VersionHistoryItem{
		persistence.NewVersionHistoryItem(int64(10), int64(100)),
	})
	rawVersionHistories := persistence.NewVersionHistories(versionHistory)
	versionHistories := rawVersionHistories.ToInternalType()
	mState := &types.GetMutableStateResponse{
		NextEventID:        11,
		CurrentBranchToken: branchToken,
		VersionHistories:   versionHistories,
	}
	s.mockHistoryClient.EXPECT().GetMutableState(gomock.Any(), gomock.Any()).Return(mState, nil).AnyTimes()

	s.mockHistoryV2Mgr.On("ReadRawHistoryBranch", mock.Anything, mock.Anything).Return(&persistence.ReadRawHistoryBranchResponse{
		HistoryEventBlobs: []*persistence.DataBlob{},
		NextPageToken:     []byte{},
		Size:              0,
	}, nil)
	_, err := s.handler.GetWorkflowExecutionRawHistoryV2(ctx,
		&types.GetWorkflowExecutionRawHistoryV2Request{
			Domain: s.domainName,
			Execution: &types.WorkflowExecution{
				WorkflowID: "workflowID",
				RunID:      uuid.New(),
			},
			StartEventID:      common.Int64Ptr(1),
			StartEventVersion: common.Int64Ptr(100),
			EndEventID:        common.Int64Ptr(10),
			EndEventVersion:   common.Int64Ptr(100),
			MaximumPageSize:   10,
			NextPageToken:     nil,
		})
	s.NoError(err)
}

func (s *adminHandlerSuite) Test_GetWorkflowExecutionRawHistoryV2_SameStartIDAndEndID() {
	ctx := context.Background()
	s.mockDomainCache.EXPECT().GetDomainID(s.domainName).Return(s.domainID, nil).AnyTimes()
	branchToken := []byte{1}
	versionHistory := persistence.NewVersionHistory(branchToken, []*persistence.VersionHistoryItem{
		persistence.NewVersionHistoryItem(int64(10), int64(100)),
	})
	rawVersionHistories := persistence.NewVersionHistories(versionHistory)
	versionHistories := rawVersionHistories.ToInternalType()
	mState := &types.GetMutableStateResponse{
		NextEventID:        11,
		CurrentBranchToken: branchToken,
		VersionHistories:   versionHistories,
	}
	s.mockHistoryClient.EXPECT().GetMutableState(gomock.Any(), gomock.Any()).Return(mState, nil).AnyTimes()

	resp, err := s.handler.GetWorkflowExecutionRawHistoryV2(ctx,
		&types.GetWorkflowExecutionRawHistoryV2Request{
			Domain: s.domainName,
			Execution: &types.WorkflowExecution{
				WorkflowID: "workflowID",
				RunID:      uuid.New(),
			},
			StartEventID:      common.Int64Ptr(10),
			StartEventVersion: common.Int64Ptr(100),
			MaximumPageSize:   1,
			NextPageToken:     nil,
		})
	s.Nil(resp.NextPageToken)
	s.NoError(err)
}

func (s *adminHandlerSuite) Test_SetRequestDefaultValueAndGetTargetVersionHistory_DefinedStartAndEnd() {
	inputStartEventID := int64(1)
	inputStartVersion := int64(10)
	inputEndEventID := int64(100)
	inputEndVersion := int64(11)
	firstItem := persistence.NewVersionHistoryItem(inputStartEventID, inputStartVersion)
	endItem := persistence.NewVersionHistoryItem(inputEndEventID, inputEndVersion)
	versionHistory := persistence.NewVersionHistory([]byte{}, []*persistence.VersionHistoryItem{firstItem, endItem})
	versionHistories := persistence.NewVersionHistories(versionHistory)
	request := &types.GetWorkflowExecutionRawHistoryV2Request{
		Domain: s.domainName,
		Execution: &types.WorkflowExecution{
			WorkflowID: "workflowID",
			RunID:      uuid.New(),
		},
		StartEventID:      common.Int64Ptr(inputStartEventID),
		StartEventVersion: common.Int64Ptr(inputStartVersion),
		EndEventID:        common.Int64Ptr(inputEndEventID),
		EndEventVersion:   common.Int64Ptr(inputEndVersion),
		MaximumPageSize:   10,
		NextPageToken:     nil,
	}

	targetVersionHistory, err := s.handler.setRequestDefaultValueAndGetTargetVersionHistory(
		request,
		versionHistories,
	)
	s.Equal(request.GetStartEventID(), inputStartEventID)
	s.Equal(request.GetEndEventID(), inputEndEventID)
	s.Equal(targetVersionHistory, versionHistory)
	s.NoError(err)
}

func (s *adminHandlerSuite) Test_SetRequestDefaultValueAndGetTargetVersionHistory_DefinedEndEvent() {
	inputStartEventID := int64(1)
	inputEndEventID := int64(100)
	inputStartVersion := int64(10)
	inputEndVersion := int64(11)
	firstItem := persistence.NewVersionHistoryItem(inputStartEventID, inputStartVersion)
	targetItem := persistence.NewVersionHistoryItem(inputEndEventID, inputEndVersion)
	versionHistory := persistence.NewVersionHistory([]byte{}, []*persistence.VersionHistoryItem{firstItem, targetItem})
	versionHistories := persistence.NewVersionHistories(versionHistory)
	request := &types.GetWorkflowExecutionRawHistoryV2Request{
		Domain: s.domainName,
		Execution: &types.WorkflowExecution{
			WorkflowID: "workflowID",
			RunID:      uuid.New(),
		},
		StartEventID:      nil,
		StartEventVersion: nil,
		EndEventID:        common.Int64Ptr(inputEndEventID),
		EndEventVersion:   common.Int64Ptr(inputEndVersion),
		MaximumPageSize:   10,
		NextPageToken:     nil,
	}

	targetVersionHistory, err := s.handler.setRequestDefaultValueAndGetTargetVersionHistory(
		request,
		versionHistories,
	)
	s.Equal(request.GetStartEventID(), inputStartEventID-1)
	s.Equal(request.GetEndEventID(), inputEndEventID)
	s.Equal(targetVersionHistory, versionHistory)
	s.NoError(err)
}

func (s *adminHandlerSuite) Test_SetRequestDefaultValueAndGetTargetVersionHistory_DefinedStartEvent() {
	inputStartEventID := int64(1)
	inputEndEventID := int64(100)
	inputStartVersion := int64(10)
	inputEndVersion := int64(11)
	firstItem := persistence.NewVersionHistoryItem(inputStartEventID, inputStartVersion)
	targetItem := persistence.NewVersionHistoryItem(inputEndEventID, inputEndVersion)
	versionHistory := persistence.NewVersionHistory([]byte{}, []*persistence.VersionHistoryItem{firstItem, targetItem})
	versionHistories := persistence.NewVersionHistories(versionHistory)
	request := &types.GetWorkflowExecutionRawHistoryV2Request{
		Domain: s.domainName,
		Execution: &types.WorkflowExecution{
			WorkflowID: "workflowID",
			RunID:      uuid.New(),
		},
		StartEventID:      common.Int64Ptr(inputStartEventID),
		StartEventVersion: common.Int64Ptr(inputStartVersion),
		EndEventID:        nil,
		EndEventVersion:   nil,
		MaximumPageSize:   10,
		NextPageToken:     nil,
	}

	targetVersionHistory, err := s.handler.setRequestDefaultValueAndGetTargetVersionHistory(
		request,
		versionHistories,
	)
	s.Equal(request.GetStartEventID(), inputStartEventID)
	s.Equal(request.GetEndEventID(), inputEndEventID+1)
	s.Equal(targetVersionHistory, versionHistory)
	s.NoError(err)
}

func (s *adminHandlerSuite) Test_SetRequestDefaultValueAndGetTargetVersionHistory_NonCurrentBranch() {
	inputStartEventID := int64(1)
	inputEndEventID := int64(100)
	inputStartVersion := int64(10)
	inputEndVersion := int64(101)
	item1 := persistence.NewVersionHistoryItem(inputStartEventID, inputStartVersion)
	item2 := persistence.NewVersionHistoryItem(inputEndEventID, inputEndVersion)
	versionHistory1 := persistence.NewVersionHistory([]byte{}, []*persistence.VersionHistoryItem{item1, item2})
	item3 := persistence.NewVersionHistoryItem(int64(10), int64(20))
	item4 := persistence.NewVersionHistoryItem(int64(20), int64(51))
	versionHistory2 := persistence.NewVersionHistory([]byte{}, []*persistence.VersionHistoryItem{item1, item3, item4})
	versionHistories := persistence.NewVersionHistories(versionHistory1)
	_, _, err := versionHistories.AddVersionHistory(versionHistory2)
	s.NoError(err)
	request := &types.GetWorkflowExecutionRawHistoryV2Request{
		Domain: s.domainName,
		Execution: &types.WorkflowExecution{
			WorkflowID: "workflowID",
			RunID:      uuid.New(),
		},
		StartEventID:      common.Int64Ptr(9),
		StartEventVersion: common.Int64Ptr(20),
		EndEventID:        common.Int64Ptr(inputEndEventID),
		EndEventVersion:   common.Int64Ptr(inputEndVersion),
		MaximumPageSize:   10,
		NextPageToken:     nil,
	}

	targetVersionHistory, err := s.handler.setRequestDefaultValueAndGetTargetVersionHistory(
		request,
		versionHistories,
	)
	s.Equal(request.GetStartEventID(), inputStartEventID)
	s.Equal(request.GetEndEventID(), inputEndEventID)
	s.Equal(targetVersionHistory, versionHistory1)
	s.NoError(err)
}

func (s *adminHandlerSuite) Test_AddSearchAttribute_Validate() {
	handler := s.handler
	handler.params = &resource.Params{}
	ctx := context.Background()

	type test struct {
		Name     string
		Request  *types.AddSearchAttributeRequest
		Expected error
	}
	// request validation tests
	testCases1 := []test{
		{
			Name:     "nil request",
			Request:  nil,
			Expected: &types.BadRequestError{Message: "Request is nil."},
		},
		{
			Name:     "empty request",
			Request:  &types.AddSearchAttributeRequest{},
			Expected: &types.BadRequestError{Message: "SearchAttributes are not provided"},
		},
	}
	for _, testCase := range testCases1 {
		s.Equal(testCase.Expected, handler.AddSearchAttribute(ctx, testCase.Request))
	}

	dynamicConfig := dynamicconfig.NewMockClient(s.controller)
	handler.params.DynamicConfig = dynamicConfig
	// add advanced visibility store related config
	handler.params.ESConfig = &config.ElasticSearchConfig{}
	esClient := &esmock.GenericClient{}
	defer func() { esClient.AssertExpectations(s.T()) }()
	handler.params.ESClient = esClient
	handler.esClient = esClient

	mockValidAttr := map[string]interface{}{
		"testkey": types.IndexedValueTypeKeyword,
	}
	dynamicConfig.EXPECT().GetMapValue(dynamicconfig.ValidSearchAttributes, nil).
		Return(mockValidAttr, nil).AnyTimes()

	testCases2 := []test{
		{
			Name: "reserved key",
			Request: &types.AddSearchAttributeRequest{
				SearchAttribute: map[string]types.IndexedValueType{
					"WorkflowID": 1,
				},
			},
			Expected: &types.BadRequestError{Message: "Key [WorkflowID] is reserved by system"},
		},
		{
			Name: "key already whitelisted",
			Request: &types.AddSearchAttributeRequest{
				SearchAttribute: map[string]types.IndexedValueType{
					"testkey": 1,
				},
			},
			Expected: &types.BadRequestError{Message: "Key [testkey] is already whitelisted as a different type"},
		},
	}
	for _, testCase := range testCases2 {
		s.Equal(testCase.Expected, handler.AddSearchAttribute(ctx, testCase.Request))
	}

	dcUpdateTest := test{
		Name: "dynamic config update failed",
		Request: &types.AddSearchAttributeRequest{
			SearchAttribute: map[string]types.IndexedValueType{
				"testkey2": -1,
			},
		},
		Expected: &types.BadRequestError{Message: "Unknown value type, IndexedValueType(-1)"},
	}
	dynamicConfig.EXPECT().UpdateValue(dynamicconfig.ValidSearchAttributes, map[string]interface{}{
		"testkey":  types.IndexedValueTypeKeyword,
		"testkey2": -1,
	}).Return(errors.New("error"))
	err := handler.AddSearchAttribute(ctx, dcUpdateTest.Request)
	s.Equal(dcUpdateTest.Expected, err)

	// ES operations tests
	dynamicConfig.EXPECT().UpdateValue(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	convertFailedTest := test{
		Name: "unknown value type",
		Request: &types.AddSearchAttributeRequest{
			SearchAttribute: map[string]types.IndexedValueType{
				"testkey3": -1,
			},
		},
		Expected: &types.BadRequestError{Message: "Unknown value type, IndexedValueType(-1)"},
	}
	s.Equal(convertFailedTest.Expected, handler.AddSearchAttribute(ctx, convertFailedTest.Request))

	esClient.On("PutMapping", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("error"))
	esClient.On("IsNotFoundError", mock.Anything).Return(false)
	esErrorTest := test{
		Name: "es error",
		Request: &types.AddSearchAttributeRequest{
			SearchAttribute: map[string]types.IndexedValueType{
				"testkey4": 1,
			},
		},
		Expected: &types.InternalServiceError{Message: "Failed to update ES mapping, err: error"},
	}
	s.Equal(esErrorTest.Expected, handler.AddSearchAttribute(ctx, esErrorTest.Request))
}

func (s *adminHandlerSuite) Test_AddSearchAttribute_Permission() {
	ctx := context.Background()
	handler := s.handler
	handler.config = &frontendcfg.Config{
		EnableAdminProtection: dynamicconfig.GetBoolPropertyFn(true),
		AdminOperationToken:   dynamicconfig.GetStringPropertyFn(dynamicconfig.AdminOperationToken.DefaultString()),
	}

	type test struct {
		Name     string
		Request  *types.AddSearchAttributeRequest
		Expected error
	}
	testCases := []test{
		{
			Name: "unknown token",
			Request: &types.AddSearchAttributeRequest{
				SecurityToken: "unknown",
			},
			Expected: validate.ErrNoPermission,
		},
		{
			Name: "correct token",
			Request: &types.AddSearchAttributeRequest{
				SecurityToken: dynamicconfig.AdminOperationToken.DefaultString(),
			},
			Expected: &types.BadRequestError{Message: "SearchAttributes are not provided"},
		},
	}
	for _, testCase := range testCases {
		s.Equal(testCase.Expected, handler.AddSearchAttribute(ctx, testCase.Request))
	}
}

func (s *adminHandlerSuite) Test_ConfigStore_NilRequest() {
	ctx := context.Background()
	handler := s.handler

	_, err := handler.GetDynamicConfig(ctx, nil)
	s.Error(err)

	err = handler.UpdateDynamicConfig(ctx, nil)
	s.Error(err)

	err = handler.RestoreDynamicConfig(ctx, nil)
	s.Error(err)
}

func (s *adminHandlerSuite) Test_ConfigStore_InvalidKey() {
	ctx := context.Background()
	handler := s.handler

	_, err := handler.GetDynamicConfig(ctx, &types.GetDynamicConfigRequest{
		ConfigName: "invalid key",
		Filters:    nil,
	})
	s.Error(err)

	err = handler.UpdateDynamicConfig(ctx, &types.UpdateDynamicConfigRequest{
		ConfigName:   "invalid key",
		ConfigValues: nil,
	})
	s.Error(err)

	err = handler.RestoreDynamicConfig(ctx, &types.RestoreDynamicConfigRequest{
		ConfigName: "invalid key",
		Filters:    nil,
	})
	s.Error(err)
}

func (s *adminHandlerSuite) Test_GetDynamicConfig_NoFilter() {
	ctx := context.Background()
	handler := s.handler
	dynamicConfig := dynamicconfig.NewMockClient(s.controller)
	handler.params.DynamicConfig = dynamicConfig

	dynamicConfig.EXPECT().
		GetValue(dynamicconfig.TestGetBoolPropertyKey).
		Return(true, nil).AnyTimes()

	resp, err := handler.GetDynamicConfig(ctx, &types.GetDynamicConfigRequest{
		ConfigName: dynamicconfig.TestGetBoolPropertyKey.String(),
		Filters:    nil,
	})
	s.NoError(err)

	encTrue, err := json.Marshal(true)
	s.NoError(err)
	s.Equal(resp.Value.Data, encTrue)
}

func (s *adminHandlerSuite) Test_GetDynamicConfig_FilterMatch() {
	ctx := context.Background()
	handler := s.handler
	dynamicConfig := dynamicconfig.NewMockClient(s.controller)
	handler.params.DynamicConfig = dynamicConfig

	dynamicConfig.EXPECT().
		GetValueWithFilters(dynamicconfig.TestGetBoolPropertyKey, map[dynamicconfig.Filter]interface{}{
			dynamicconfig.DomainName: "samples_domain",
		}).
		Return(true, nil).AnyTimes()

	encDomainName, err := json.Marshal("samples_domain")
	s.NoError(err)

	resp, err := handler.GetDynamicConfig(ctx, &types.GetDynamicConfigRequest{
		ConfigName: dynamicconfig.TestGetBoolPropertyKey.String(),
		Filters: []*types.DynamicConfigFilter{
			{
				Name: dynamicconfig.DomainName.String(),
				Value: &types.DataBlob{
					EncodingType: types.EncodingTypeJSON.Ptr(),
					Data:         encDomainName,
				},
			},
		},
	})
	s.NoError(err)

	encTrue, err := json.Marshal(true)
	s.NoError(err)
	s.Equal(resp.Value.Data, encTrue)
}

func Test_GetGlobalIsolationGroups(t *testing.T) {

	validResponse := types.GetGlobalIsolationGroupsResponse{
		IsolationGroups: types.IsolationGroupConfiguration{
			"zone-1": types.IsolationGroupPartition{
				Name:  "zone-1",
				State: types.IsolationGroupStateDrained,
			},
			"zone-2": types.IsolationGroupPartition{
				Name:  "zone-2",
				State: types.IsolationGroupStateHealthy,
			},
			"zone-3": types.IsolationGroupPartition{
				Name:  "zone-3",
				State: types.IsolationGroupStateDrained,
			},
		},
	}

	tests := map[string]struct {
		ighandlerAffordance func(mock *isolationgroupapi.MockHandler)
		expectOut           *types.GetGlobalIsolationGroupsResponse
		expectedErr         error
	}{
		"happy-path - no errors and payload is decoded and returned": {
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().GetGlobalState(gomock.Any()).Return(&validResponse, nil)
			},
			expectOut: &validResponse,
		},
		"an error returned": {
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().GetGlobalState(gomock.Any()).Return(nil, assert.AnError)
			},
			expectedErr: &types.InternalServiceError{Message: assert.AnError.Error()},
		},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {

			goMock := gomock.NewController(t)
			igMock := isolationgroupapi.NewMockHandler(goMock)
			td.ighandlerAffordance(igMock)

			handler := adminHandlerImpl{
				Resource: &resource.Test{
					Logger:        testlogger.New(t),
					MetricsClient: metrics.NewNoopMetricsClient(),
				},
				isolationGroups: igMock,
			}

			res, err := handler.GetGlobalIsolationGroups(context.Background(), &types.GetGlobalIsolationGroupsRequest{})

			assert.Equal(t, td.expectOut, res)
			assert.Equal(t, td.expectedErr, err)
		})
	}
}

func Test_UpdateGlobalIsolationGroups(t *testing.T) {

	validConfig := types.UpdateGlobalIsolationGroupsRequest{
		IsolationGroups: types.IsolationGroupConfiguration{
			"zone-1": {
				Name:  "zone-1",
				State: types.IsolationGroupStateDrained,
			},
			"zone-2": {
				Name:  "zone-2",
				State: types.IsolationGroupStateHealthy,
			},
			"zone-3": {
				Name:  "zone-3",
				State: types.IsolationGroupStateDrained,
			},
		},
	}

	tests := map[string]struct {
		ighandlerAffordance func(mock *isolationgroupapi.MockHandler)
		input               *types.UpdateGlobalIsolationGroupsRequest
		expectOut           *types.UpdateGlobalIsolationGroupsResponse
		expectedErr         error
	}{
		"happy-path - update to the database": {
			input: &validConfig,
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().UpdateGlobalState(gomock.Any(), validConfig).Return(nil)
			},
			expectOut: &types.UpdateGlobalIsolationGroupsResponse{},
		},
		"happy-path - an error is returned": {
			input: &validConfig,
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().UpdateGlobalState(gomock.Any(), validConfig).Return(assert.AnError)
			},
			expectedErr: &types.InternalServiceError{Message: assert.AnError.Error()},
		},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			goMock := gomock.NewController(t)
			igMock := isolationgroupapi.NewMockHandler(goMock)
			td.ighandlerAffordance(igMock)

			handler := adminHandlerImpl{
				Resource: &resource.Test{
					Logger:        testlogger.New(t),
					MetricsClient: metrics.NewNoopMetricsClient(),
				},
				isolationGroups: igMock,
			}

			res, err := handler.UpdateGlobalIsolationGroups(context.Background(), td.input)

			assert.Equal(t, td.expectOut, res)
			assert.Equal(t, td.expectedErr, err)
		})
	}
}

func Test_IsolationGroupsNotEnabled(t *testing.T) {
	handler := adminHandlerImpl{
		Resource: &resource.Test{
			Logger:        testlogger.New(t),
			MetricsClient: metrics.NewNoopMetricsClient(),
		},
		isolationGroups: nil, // valid state, the isolation-groups feature is not available for all persistence types
	}

	_, err := handler.GetGlobalIsolationGroups(context.Background(), &types.GetGlobalIsolationGroupsRequest{})
	assert.ErrorAs(t, err, &partition.ErrNoIsolationGroupsAvailable)
	_, err = handler.UpdateGlobalIsolationGroups(context.Background(), &types.UpdateGlobalIsolationGroupsRequest{})
	assert.ErrorAs(t, err, &partition.ErrNoIsolationGroupsAvailable)
}

func Test_GetDomainIsolationGroups(t *testing.T) {

	validResponse := types.GetDomainIsolationGroupsResponse{
		IsolationGroups: types.IsolationGroupConfiguration{
			"zone-1": types.IsolationGroupPartition{
				Name:  "zone-1",
				State: types.IsolationGroupStateDrained,
			},
			"zone-2": types.IsolationGroupPartition{
				Name:  "zone-2",
				State: types.IsolationGroupStateHealthy,
			},
			"zone-3": types.IsolationGroupPartition{
				Name:  "zone-3",
				State: types.IsolationGroupStateDrained,
			},
		},
	}

	tests := map[string]struct {
		ighandlerAffordance func(mock *isolationgroupapi.MockHandler)
		expectOut           *types.GetDomainIsolationGroupsResponse
		expectedErr         error
	}{
		"happy-path - no errors and payload is decoded and returned": {
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().GetDomainState(gomock.Any(), types.GetDomainIsolationGroupsRequest{
					Domain: "domain",
				}).Return(&validResponse, nil)
			},
			expectOut: &validResponse,
		},
		"an error returned": {
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().GetDomainState(gomock.Any(), types.GetDomainIsolationGroupsRequest{
					Domain: "domain",
				}).Return(nil, assert.AnError)
			},
			expectedErr: &types.InternalServiceError{Message: assert.AnError.Error()},
		},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {

			goMock := gomock.NewController(t)
			igMock := isolationgroupapi.NewMockHandler(goMock)
			td.ighandlerAffordance(igMock)

			handler := adminHandlerImpl{
				Resource: &resource.Test{
					Logger:        testlogger.New(t),
					MetricsClient: metrics.NewNoopMetricsClient(),
				},
				isolationGroups: igMock,
			}

			res, err := handler.GetDomainIsolationGroups(context.Background(), &types.GetDomainIsolationGroupsRequest{
				Domain: "domain",
			})

			assert.Equal(t, td.expectOut, res)
			assert.Equal(t, td.expectedErr, err)
		})
	}
}

func Test_UpdateDomainIsolationGroups(t *testing.T) {

	validConfig := types.UpdateDomainIsolationGroupsRequest{
		Domain: "domain",
		IsolationGroups: types.IsolationGroupConfiguration{
			"zone-1": {
				Name:  "zone-1",
				State: types.IsolationGroupStateDrained,
			},
			"zone-2": {
				Name:  "zone-2",
				State: types.IsolationGroupStateHealthy,
			},
			"zone-3": {
				Name:  "zone-3",
				State: types.IsolationGroupStateDrained,
			},
		},
	}

	tests := map[string]struct {
		ighandlerAffordance func(mock *isolationgroupapi.MockHandler)
		input               *types.UpdateDomainIsolationGroupsRequest
		expectOut           *types.UpdateDomainIsolationGroupsResponse
		expectedErr         error
	}{
		"happy-path - update to the database": {
			input: &validConfig,
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().UpdateDomainState(gomock.Any(), validConfig).Return(nil)
			},
			expectOut: &types.UpdateDomainIsolationGroupsResponse{},
		},
		"happy-path - an error is returned": {
			input: &validConfig,
			ighandlerAffordance: func(mock *isolationgroupapi.MockHandler) {
				mock.EXPECT().UpdateDomainState(gomock.Any(), validConfig).Return(assert.AnError)
			},
			expectedErr: &types.InternalServiceError{Message: assert.AnError.Error()},
		},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			goMock := gomock.NewController(t)
			igMock := isolationgroupapi.NewMockHandler(goMock)
			td.ighandlerAffordance(igMock)

			handler := adminHandlerImpl{
				Resource: &resource.Test{
					Logger:        testlogger.New(t),
					MetricsClient: metrics.NewNoopMetricsClient(),
				},
				isolationGroups: igMock,
			}

			res, err := handler.UpdateDomainIsolationGroups(context.Background(), td.input)

			assert.Equal(t, td.expectOut, res)
			assert.Equal(t, td.expectedErr, err)
		})
	}
}

func Test_GetDomainAsyncWorkflowConfiguraton(t *testing.T) {
	tests := map[string]struct {
		queueCfgHandlerMockFn func(mock *queueconfigapi.MockHandler)
		input                 *types.GetDomainAsyncWorkflowConfiguratonRequest
		wantResp              *types.GetDomainAsyncWorkflowConfiguratonResponse
		wantErr               error
	}{
		"success": {
			input: &types.GetDomainAsyncWorkflowConfiguratonRequest{Domain: "test-domain"},
			queueCfgHandlerMockFn: func(mock *queueconfigapi.MockHandler) {
				mock.EXPECT().GetConfiguraton(gomock.Any(), gomock.Any()).Return(&types.GetDomainAsyncWorkflowConfiguratonResponse{}, nil).Times(1)
			},
			wantResp: &types.GetDomainAsyncWorkflowConfiguratonResponse{},
		},
		"nil request": {
			input:   nil,
			wantErr: validate.ErrRequestNotSet,
		},
		"queue config handler failed": {
			input: &types.GetDomainAsyncWorkflowConfiguratonRequest{Domain: "test-domain"},
			queueCfgHandlerMockFn: func(mock *queueconfigapi.MockHandler) {
				mock.EXPECT().GetConfiguraton(gomock.Any(), gomock.Any()).Return(nil, errors.New("failed")).Times(1)
			},
			wantErr: &types.InternalServiceError{Message: "failed"},
		},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			goMock := gomock.NewController(t)
			queueCfgHandlerMock := queueconfigapi.NewMockHandler(goMock)
			if td.queueCfgHandlerMockFn != nil {
				td.queueCfgHandlerMockFn(queueCfgHandlerMock)
			}

			handler := adminHandlerImpl{
				Resource: &resource.Test{
					Logger:        testlogger.New(t),
					MetricsClient: metrics.NewNoopMetricsClient(),
				},
				asyncWFQueueConfigs: queueCfgHandlerMock,
			}

			res, err := handler.GetDomainAsyncWorkflowConfiguraton(context.Background(), td.input)

			assert.Equal(t, td.wantResp, res)
			assert.Equal(t, td.wantErr, err)
		})
	}
}

func Test_UpdateDomainAsyncWorkflowConfiguraton(t *testing.T) {
	tests := map[string]struct {
		queueCfgHandlerMockFn func(mock *queueconfigapi.MockHandler)
		input                 *types.UpdateDomainAsyncWorkflowConfiguratonRequest
		wantResp              *types.UpdateDomainAsyncWorkflowConfiguratonResponse
		wantErr               error
	}{
		"success": {
			input: &types.UpdateDomainAsyncWorkflowConfiguratonRequest{Domain: "test-domain"},
			queueCfgHandlerMockFn: func(mock *queueconfigapi.MockHandler) {
				mock.EXPECT().UpdateConfiguration(gomock.Any(), gomock.Any()).Return(&types.UpdateDomainAsyncWorkflowConfiguratonResponse{}, nil).Times(1)
			},
			wantResp: &types.UpdateDomainAsyncWorkflowConfiguratonResponse{},
		},
		"nil request": {
			input:   nil,
			wantErr: validate.ErrRequestNotSet,
		},
		"queue config handler failed": {
			input: &types.UpdateDomainAsyncWorkflowConfiguratonRequest{Domain: "test-domain"},
			queueCfgHandlerMockFn: func(mock *queueconfigapi.MockHandler) {
				mock.EXPECT().UpdateConfiguration(gomock.Any(), gomock.Any()).Return(nil, errors.New("failed")).Times(1)
			},
			wantErr: &types.InternalServiceError{Message: "failed"},
		},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			goMock := gomock.NewController(t)
			queueCfgHandlerMock := queueconfigapi.NewMockHandler(goMock)
			if td.queueCfgHandlerMockFn != nil {
				td.queueCfgHandlerMockFn(queueCfgHandlerMock)
			}

			handler := adminHandlerImpl{
				Resource: &resource.Test{
					Logger:        testlogger.New(t),
					MetricsClient: metrics.NewNoopMetricsClient(),
				},
				asyncWFQueueConfigs: queueCfgHandlerMock,
			}

			res, err := handler.UpdateDomainAsyncWorkflowConfiguraton(context.Background(), td.input)

			assert.Equal(t, td.wantResp, res)
			assert.Equal(t, td.wantErr, err)
		})
	}
}
