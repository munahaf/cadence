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

package pinot

import (
	"fmt"
	"testing"
	"time"

	"github.com/startreedata/pinot-client-go/pinot"
	"github.com/stretchr/testify/assert"

	p "github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

var (
	testIndex        = "test-index"
	testDomain       = "test-domain"
	testDomainID     = "bfd5c907-f899-4baf-a7b2-2ab85e623ebd"
	testPageSize     = 10
	testEarliestTime = int64(1547596872371000000)
	testLatestTime   = int64(2547596872371000000)
	testWorkflowType = "test-wf-type"
	testWorkflowID   = "test-wid"
	testCloseStatus  = int32(1)

	client = PinotClient{
		client: nil,
		logger: nil,
	}
)

func TestBuildMap(t *testing.T) {
	columnName := []string{"WorkflowID", "RunID", "WorkflowType", "DomainID", "StartTime", "ExecutionTime", "CloseTime", "CloseStatus", "HistoryLength", "TaskList", "IsCron", "NumClusters", "UpdateTime", "CustomIntField", "CustomStringField"}
	hit := []interface{}{"wfid", "rid", "wftype", "domainid", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "tsklst", true, 1, testEarliestTime, 1, "some string"}

	tests := map[string]struct {
		inputColumnNames []string
		inputHit         []interface{}
		expectedMap      map[string]interface{}
	}{
		"Case1: with everything": {
			inputColumnNames: columnName,
			inputHit:         hit,
			expectedMap:      map[string]interface{}{"CloseStatus": 1, "CloseTime": int64(2547596872371000000), "CustomIntField": 1, "CustomStringField": "some string", "DomainID": "domainid", "ExecutionTime": int64(1547596872371000000), "HistoryLength": 1, "IsCron": true, "NumClusters": 1, "RunID": "rid", "StartTime": int64(1547596872371000000), "TaskList": "tsklst", "UpdateTime": int64(1547596872371000000), "WorkflowID": "wfid", "WorkflowType": "wftype"},
		},
		"Case2: nil result": {
			inputColumnNames: nil,
			inputHit:         nil,
			expectedMap:      map[string]interface{}{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				resMap := buildMap(test.inputHit, test.inputColumnNames)
				assert.Equal(t, test.expectedMap, resMap)
			})
		})
	}
}

func TestConvertSearchResultToVisibilityRecord(t *testing.T) {
	columnName := []string{"WorkflowID", "RunID", "WorkflowType", "DomainID", "StartTime", "ExecutionTime", "CloseTime", "CloseStatus", "HistoryLength", "TaskList", "IsCron", "NumClusters", "UpdateTime", "Attr"}
	hit := []interface{}{"wfid", "rid", "wftype", "domainid", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "tsklst", true, 1, testEarliestTime, "{}"}
	closeStatus := types.WorkflowExecutionCloseStatusFailed

	columnNameWithAttr := []string{"WorkflowID", "RunID", "WorkflowType", "DomainID", "StartTime", "ExecutionTime", "CloseTime", "CloseStatus", "HistoryLength", "TaskList", "IsCron", "NumClusters", "UpdateTime", "Attr"}
	hitWithAttr := []interface{}{"wfid", "rid", "wftype", "domainid", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "tsklst", true, 1, testEarliestTime, `{"CustomStringField": "customA and customB or customC", "CustomDoubleField": 3.14}`}

	tests := map[string]struct {
		inputColumnNames         []string
		inputHit                 []interface{}
		expectedVisibilityRecord *p.InternalVisibilityWorkflowExecutionInfo
	}{
		"Case1: with everything except for an empty Attr": {
			inputColumnNames: columnName,
			inputHit:         hit,
			expectedVisibilityRecord: &p.InternalVisibilityWorkflowExecutionInfo{
				DomainID:         "domainid",
				WorkflowType:     "wftype",
				WorkflowID:       "wfid",
				RunID:            "rid",
				TypeName:         "wftype",
				StartTime:        time.UnixMilli(testEarliestTime),
				ExecutionTime:    time.UnixMilli(testEarliestTime),
				CloseTime:        time.UnixMilli(testLatestTime),
				Status:           &closeStatus,
				HistoryLength:    1,
				Memo:             nil,
				TaskList:         "tsklst",
				IsCron:           true,
				NumClusters:      1,
				UpdateTime:       time.UnixMilli(testEarliestTime),
				SearchAttributes: map[string]interface{}{},
				ShardID:          0,
			},
		},
		"Case2: with everything": {
			inputColumnNames: columnNameWithAttr,
			inputHit:         hitWithAttr,
			expectedVisibilityRecord: &p.InternalVisibilityWorkflowExecutionInfo{
				DomainID:         "domainid",
				WorkflowType:     "wftype",
				WorkflowID:       "wfid",
				RunID:            "rid",
				TypeName:         "wftype",
				StartTime:        time.UnixMilli(testEarliestTime),
				ExecutionTime:    time.UnixMilli(testEarliestTime),
				CloseTime:        time.UnixMilli(testLatestTime),
				Status:           &closeStatus,
				HistoryLength:    1,
				Memo:             nil,
				TaskList:         "tsklst",
				IsCron:           true,
				NumClusters:      1,
				UpdateTime:       time.UnixMilli(testEarliestTime),
				SearchAttributes: map[string]interface{}{"CustomStringField": "customA and customB or customC", "CustomDoubleField": 3.14},
				ShardID:          0,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				visibilityRecord := ConvertSearchResultToVisibilityRecord(test.inputHit, test.inputColumnNames, nil)
				assert.Equal(t, test.expectedVisibilityRecord, visibilityRecord)
			})
		})
	}
}

func TestGetInternalListWorkflowExecutionsResponse(t *testing.T) {
	columnName := []string{"WorkflowID", "RunID", "WorkflowType", "DomainID", "StartTime", "ExecutionTime", "CloseTime", "CloseStatus", "HistoryLength", "Encoding", "TaskList", "IsCron", "NumClusters", "UpdateTime", "Attr"}
	hit1 := []interface{}{"wfid1", "rid1", "wftype1", "domainid1", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "encode1", "tsklst1", true, 1, testEarliestTime, "null"}
	hit2 := []interface{}{"wfid2", "rid2", "wftype2", "domainid2", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "encode2", "tsklst2", false, 1, testEarliestTime, "null"}
	hit3 := []interface{}{"wfid3", "rid3", "wftype3", "domainid3", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "encode3", "tsklst3", false, 1, testEarliestTime, "null"}
	hit4 := []interface{}{"wfid4", "rid4", "wftype4", "domainid4", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "encode4", "tsklst4", false, 1, testEarliestTime, "null"}
	hit5 := []interface{}{"wfid5", "rid5", "wftype5", "domainid5", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "encode5", "tsklst5", false, 1, testEarliestTime, "null"}

	brokerResponse := &pinot.BrokerResponse{
		AggregationResults: nil,
		SelectionResults:   nil,
		ResultTable: &pinot.ResultTable{
			DataSchema: pinot.RespSchema{
				ColumnDataTypes: nil,
				ColumnNames:     columnName,
			},
			Rows: [][]interface{}{
				hit1,
				hit2,
				hit3,
				hit4,
				hit5,
			},
		},
		Exceptions:                  nil,
		TraceInfo:                   nil,
		NumServersQueried:           1,
		NumServersResponded:         1,
		NumSegmentsQueried:          1,
		NumSegmentsProcessed:        1,
		NumSegmentsMatched:          1,
		NumConsumingSegmentsQueried: 1,
		NumDocsScanned:              10,
		NumEntriesScannedInFilter:   1,
		NumEntriesScannedPostFilter: 1,
		NumGroupsLimitReached:       false,
		TotalDocs:                   1,
		TimeUsedMs:                  1,
		MinConsumingFreshnessTimeMs: 1,
	}

	token := &PinotVisibilityPageToken{
		From: 0,
	}

	// Cannot use a table test, because they are not checking the same fields
	result, err := client.getInternalListWorkflowExecutionsResponse(brokerResponse, nil, token, 5, 33)

	assert.Equal(t, "wfid1", result.Executions[0].WorkflowID)
	assert.Equal(t, "rid1", result.Executions[0].RunID)
	assert.Equal(t, "wftype1", result.Executions[0].WorkflowType)
	assert.Equal(t, "domainid1", result.Executions[0].DomainID)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Executions[0].StartTime)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Executions[0].ExecutionTime)
	assert.Equal(t, time.UnixMilli(testLatestTime), result.Executions[0].CloseTime)
	assert.Equal(t, types.WorkflowExecutionCloseStatus(1), *result.Executions[0].Status)
	assert.Equal(t, int64(1), result.Executions[0].HistoryLength)
	assert.Equal(t, "tsklst1", result.Executions[0].TaskList)
	assert.Equal(t, true, result.Executions[0].IsCron)
	assert.Equal(t, int16(1), result.Executions[0].NumClusters)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Executions[0].UpdateTime)

	assert.Equal(t, "wfid2", result.Executions[1].WorkflowID)
	assert.Equal(t, "rid2", result.Executions[1].RunID)
	assert.Equal(t, "wftype2", result.Executions[1].WorkflowType)
	assert.Equal(t, "domainid2", result.Executions[1].DomainID)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Executions[1].StartTime)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Executions[1].ExecutionTime)
	assert.Equal(t, time.UnixMilli(testLatestTime), result.Executions[1].CloseTime)
	assert.Equal(t, types.WorkflowExecutionCloseStatus(1), *result.Executions[1].Status)
	assert.Equal(t, int64(1), result.Executions[1].HistoryLength)
	assert.Equal(t, "tsklst2", result.Executions[1].TaskList)
	assert.Equal(t, false, result.Executions[1].IsCron)
	assert.Equal(t, int16(1), result.Executions[1].NumClusters)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Executions[1].UpdateTime)

	assert.Nil(t, err)

	responseToken := result.NextPageToken
	unmarshalResponseToken, err := GetNextPageToken(responseToken)
	if err != nil {
		panic(fmt.Sprintf("Unmarshal error in PinotClient test %s", err))
	}
	assert.Equal(t, 5, unmarshalResponseToken.From)

	// check if record is not valid
	isRecordValid := func(rec *p.InternalVisibilityWorkflowExecutionInfo) bool {
		return false
	}
	emptyResult, err := client.getInternalListWorkflowExecutionsResponse(brokerResponse, isRecordValid, nil, 10, 33)
	assert.Equal(t, 0, len(emptyResult.Executions))
	assert.Nil(t, err)

	// check nil input
	nilResult, err := client.getInternalListWorkflowExecutionsResponse(nil, isRecordValid, nil, 10, 33)
	assert.Equal(t, &p.InternalListWorkflowExecutionsResponse{}, nilResult)
	assert.Nil(t, err)
}

func TestGetInternalGetClosedWorkflowExecutionResponse(t *testing.T) {
	columnName := []string{"WorkflowID", "RunID", "WorkflowType", "DomainID", "StartTime", "ExecutionTime", "CloseTime", "CloseStatus", "HistoryLength", "Encoding", "TaskList", "IsCron", "NumClusters", "UpdateTime", "Attr"}
	hit1 := []interface{}{"wfid1", "rid1", "wftype1", "domainid1", testEarliestTime, testEarliestTime, testLatestTime, 1, 1, "encode1", "tsklst1", true, 1, testEarliestTime, "null"}

	brokerResponse := &pinot.BrokerResponse{
		AggregationResults: nil,
		SelectionResults:   nil,
		ResultTable: &pinot.ResultTable{
			DataSchema: pinot.RespSchema{
				ColumnDataTypes: nil,
				ColumnNames:     columnName,
			},
			Rows: [][]interface{}{
				hit1,
			},
		},
		Exceptions:                  nil,
		TraceInfo:                   nil,
		NumServersQueried:           1,
		NumServersResponded:         1,
		NumSegmentsQueried:          1,
		NumSegmentsProcessed:        1,
		NumSegmentsMatched:          1,
		NumConsumingSegmentsQueried: 1,
		NumDocsScanned:              1,
		NumEntriesScannedInFilter:   1,
		NumEntriesScannedPostFilter: 1,
		NumGroupsLimitReached:       false,
		TotalDocs:                   1,
		TimeUsedMs:                  1,
		MinConsumingFreshnessTimeMs: 1,
	}

	result, err := client.getInternalGetClosedWorkflowExecutionResponse(brokerResponse)

	assert.Equal(t, "wfid1", result.Execution.WorkflowID)
	assert.Equal(t, "rid1", result.Execution.RunID)
	assert.Equal(t, "wftype1", result.Execution.WorkflowType)
	assert.Equal(t, "domainid1", result.Execution.DomainID)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Execution.StartTime)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Execution.ExecutionTime)
	assert.Equal(t, time.UnixMilli(testLatestTime), result.Execution.CloseTime)
	assert.Equal(t, types.WorkflowExecutionCloseStatus(1), *result.Execution.Status)
	assert.Equal(t, int64(1), result.Execution.HistoryLength)
	assert.Equal(t, "tsklst1", result.Execution.TaskList)
	assert.Equal(t, true, result.Execution.IsCron)
	assert.Equal(t, int16(1), result.Execution.NumClusters)
	assert.Equal(t, time.UnixMilli(testEarliestTime), result.Execution.UpdateTime)

	assert.Nil(t, err)
}
