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

package persistenceutils

import (
	"context"
	"errors"

	"github.com/uber/cadence/common/cache"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/types"
)

// ReadFullPageV2Events reads a full page of history events from HistoryManager. Due to storage format of V2 History
// it is not guaranteed that pageSize amount of data is returned. Function returns the list of history events, the size
// of data read, the next page token, and an error if present.
func ReadFullPageV2Events(
	ctx context.Context,
	historyV2Mgr persistence.HistoryManager,
	req *persistence.ReadHistoryBranchRequest,
) ([]*types.HistoryEvent, int, []byte, error) {
	historyEvents := []*types.HistoryEvent{}
	size := int(0)
	for {
		response, err := historyV2Mgr.ReadHistoryBranch(ctx, req)
		if err != nil {
			return nil, 0, nil, err
		}
		historyEvents = append(historyEvents, response.HistoryEvents...)
		size += response.Size
		if len(historyEvents) >= req.PageSize || len(response.NextPageToken) == 0 {
			return historyEvents, size, response.NextPageToken, nil
		}
		req.NextPageToken = response.NextPageToken
	}
}

// ReadFullPageV2EventsByBatch reads a full page of history events by batch from HistoryManager. Due to storage format of V2 History
// it is not guaranteed that pageSize amount of data is returned. Function returns the list of history batches, the size
// of data read, the next page token, and an error if present.
func ReadFullPageV2EventsByBatch(
	ctx context.Context,
	historyV2Mgr persistence.HistoryManager,
	req *persistence.ReadHistoryBranchRequest,
) ([]*types.History, int, []byte, error) {
	historyBatches := []*types.History{}
	eventsRead := 0
	size := 0
	for {
		response, err := historyV2Mgr.ReadHistoryBranchByBatch(ctx, req)
		if err != nil {
			return nil, 0, nil, err
		}
		historyBatches = append(historyBatches, response.History...)
		for _, batch := range response.History {
			eventsRead += len(batch.Events)
		}
		size += response.Size
		if eventsRead >= req.PageSize || len(response.NextPageToken) == 0 {
			return historyBatches, size, response.NextPageToken, nil
		}
		req.NextPageToken = response.NextPageToken
	}
}

// GetBeginNodeID gets node id from last ancestor
func GetBeginNodeID(bi types.HistoryBranch) int64 {
	if len(bi.Ancestors) == 0 {
		// root branch
		return 1
	}
	idx := len(bi.Ancestors) - 1
	return bi.Ancestors[idx].EndNodeID
}

// PaginateHistory return paged history
func PaginateHistory(
	ctx context.Context,
	historyV2Mgr persistence.HistoryManager,
	byBatch bool,
	branchToken []byte,
	firstEventID int64,
	nextEventID int64,
	tokenIn []byte,
	pageSize int,
	shardID *int,
	domainID string,
	domainCache cache.DomainCache,
) ([]*types.HistoryEvent, []*types.History, []byte, int, error) {

	historyEvents := []*types.HistoryEvent{}
	historyBatches := []*types.History{}
	var tokenOut []byte
	var historySize int
	domainName, err := domainCache.GetDomainName(domainID)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	req := &persistence.ReadHistoryBranchRequest{
		BranchToken:   branchToken,
		MinEventID:    firstEventID,
		MaxEventID:    nextEventID,
		PageSize:      pageSize,
		NextPageToken: tokenIn,
		ShardID:       shardID,
		DomainName:    domainName,
	}
	if byBatch {
		response, err := historyV2Mgr.ReadHistoryBranchByBatch(ctx, req)
		if err != nil {
			var e *types.EntityNotExistsError
			if errors.As(err, &e) {
				return nil, nil, nil, 0, nil
			}
			return nil, nil, nil, 0, err
		}

		// Keep track of total history size
		historySize += response.Size
		historyBatches = append(historyBatches, response.History...)
		tokenOut = response.NextPageToken

	} else {
		response, err := historyV2Mgr.ReadHistoryBranch(ctx, req)
		if err != nil {
			var e *types.EntityNotExistsError
			if errors.As(err, &e) {
				return nil, nil, nil, 0, nil
			}
			return nil, nil, nil, 0, err
		}

		// Keep track of total history size
		historySize += response.Size
		historyEvents = append(historyEvents, response.HistoryEvents...)
		tokenOut = response.NextPageToken
	}

	return historyEvents, historyBatches, tokenOut, historySize, nil
}

// Get the maximum referenced node id of each branch
func GetBranchesMaxReferredNodeIDs(branches []*types.HistoryBranch) map[string]int64 {
	validBRsMaxEndNode := map[string]int64{}
	for _, b := range branches {
		for _, br := range b.Ancestors {
			curr, ok := validBRsMaxEndNode[br.BranchID]
			if !ok || curr < br.EndNodeID {
				validBRsMaxEndNode[br.BranchID] = br.EndNodeID
			}
		}
	}
	return validBRsMaxEndNode
}
