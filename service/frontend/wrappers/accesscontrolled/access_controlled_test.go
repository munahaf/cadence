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

package accesscontrolled

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/uber/cadence/common/authorization"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/metrics/mocks"
)

func TestIsAuthorized(t *testing.T) {
	testCases := []struct {
		name         string
		mockSetup    func(*authorization.MockAuthorizer, *mocks.Scope)
		isAuthorized bool
		wantErr      bool
	}{
		{
			name: "Succes case",
			mockSetup: func(authorizer *authorization.MockAuthorizer, scope *mocks.Scope) {
				authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any()).Return(authorization.Result{Decision: authorization.DecisionAllow}, nil)
				scope.On("StartTimer", metrics.CadenceAuthorizationLatency).Return(metrics.NewTestStopwatch()).Once()
			},
			isAuthorized: true,
			wantErr:      false,
		},
		{
			name: "Error case - unauthorized",
			mockSetup: func(authorizer *authorization.MockAuthorizer, scope *mocks.Scope) {
				authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any()).Return(authorization.Result{Decision: authorization.DecisionDeny}, nil)
				scope.On("StartTimer", metrics.CadenceAuthorizationLatency).Return(metrics.NewTestStopwatch()).Once()
				scope.On("IncCounter", metrics.CadenceErrUnauthorizedCounter).Return().Once()
			},
			isAuthorized: false,
			wantErr:      false,
		},
		{
			name: "Error case - authorization error",
			mockSetup: func(authorizer *authorization.MockAuthorizer, scope *mocks.Scope) {
				authorizer.EXPECT().Authorize(gomock.Any(), gomock.Any()).Return(authorization.Result{}, errors.New("some random error"))
				scope.On("StartTimer", metrics.CadenceAuthorizationLatency).Return(metrics.NewTestStopwatch()).Once()
				scope.On("IncCounter", metrics.CadenceErrAuthorizeFailedCounter).Return().Once()
			},
			isAuthorized: false,
			wantErr:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			controller := gomock.NewController(t)

			mockAuthorizer := authorization.NewMockAuthorizer(controller)
			mockMetricsScope := &mocks.Scope{}
			tc.mockSetup(mockAuthorizer, mockMetricsScope)

			handler := &apiHandler{authorizer: mockAuthorizer}
			got, err := handler.isAuthorized(context.Background(), &authorization.Attributes{}, mockMetricsScope)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.isAuthorized, got)
			}
		})
	}
}
