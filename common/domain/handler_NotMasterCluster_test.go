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

package domain

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/archiver"
	"github.com/uber/cadence/common/archiver/provider"
	"github.com/uber/cadence/common/clock"
	"github.com/uber/cadence/common/cluster"
	"github.com/uber/cadence/common/config"
	dc "github.com/uber/cadence/common/dynamicconfig"
	"github.com/uber/cadence/common/mocks"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/persistence/nosql/nosqlplugin/cassandra/gocql/public"
	persistencetests "github.com/uber/cadence/common/persistence/persistence-tests"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/testflags"
)

type (
	domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite struct {
		*persistencetests.TestBase

		minRetentionDays     int
		maxBadBinaryCount    int
		domainManager        persistence.DomainManager
		mockProducer         *mocks.KafkaProducer
		mockDomainReplicator Replicator
		archivalMetadata     archiver.ArchivalMetadata
		mockArchiverProvider *provider.MockArchiverProvider

		handler *handlerImpl
	}
)

func TestDomainHandlerGlobalDomainEnabledNotPrimaryClusterSuite(t *testing.T) {
	testflags.RequireCassandra(t)

	if testing.Verbose() {
		log.SetOutput(os.Stdout)
	}

	s := new(domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite)

	s.TestBase = public.NewTestBaseWithPublicCassandra(t, &persistencetests.TestBaseOptions{
		ClusterMetadata: cluster.GetTestClusterMetadata(false),
	})

	suite.Run(t, s)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TearDownSuite() {
	s.TestBase.TearDownWorkflowStore()
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) SetupTest() {
	s.Setup()

	logger := s.Logger
	dcCollection := dc.NewCollection(dc.NewNopClient(), logger)
	s.minRetentionDays = 1
	s.maxBadBinaryCount = 10
	s.domainManager = s.TestBase.DomainManager
	s.mockProducer = &mocks.KafkaProducer{}
	s.mockDomainReplicator = NewDomainReplicator(s.mockProducer, logger)
	s.archivalMetadata = archiver.NewArchivalMetadata(
		dcCollection,
		"",
		false,
		"",
		false,
		&config.ArchivalDomainDefaults{},
	)
	s.mockArchiverProvider = &provider.MockArchiverProvider{}
	domainConfig := Config{
		MinRetentionDays:  dc.GetIntPropertyFn(s.minRetentionDays),
		MaxBadBinaryCount: dc.GetIntPropertyFilteredByDomain(s.maxBadBinaryCount),
		FailoverCoolDown:  dc.GetDurationPropertyFnFilteredByDomain(0 * time.Second),
	}
	s.handler = NewHandler(
		domainConfig,
		logger,
		s.domainManager,
		s.ClusterMetadata,
		s.mockDomainReplicator,
		s.archivalMetadata,
		s.mockArchiverProvider,
		clock.NewRealTimeSource(),
	).(*handlerImpl)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TearDownTest() {
	s.mockProducer.AssertExpectations(s.T())
	s.mockArchiverProvider.AssertExpectations(s.T())
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestRegisterGetDomain_LocalDomain_AllDefault() {
	domainName := s.getRandomDomainName()
	isGlobalDomain := false
	var clusters []*types.ClusterReplicationConfiguration
	for _, replicationConfig := range cluster.GetOrUseDefaultClusters(s.ClusterMetadata.GetCurrentClusterName(), nil) {
		clusters = append(clusters, &types.ClusterReplicationConfiguration{
			ClusterName: replicationConfig.ClusterName,
		})
	}

	retention := int32(1)
	err := s.handler.RegisterDomain(context.Background(), &types.RegisterDomainRequest{
		Name:                                   domainName,
		IsGlobalDomain:                         isGlobalDomain,
		WorkflowExecutionRetentionPeriodInDays: retention,
	})
	s.Nil(err)

	resp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.Nil(err)

	s.NotEmpty(resp.DomainInfo.GetUUID())
	resp.DomainInfo.UUID = ""
	s.Equal(&types.DomainInfo{
		Name:        domainName,
		Status:      types.DomainStatusRegistered.Ptr(),
		Description: "",
		OwnerEmail:  "",
		Data:        map[string]string{},
		UUID:        "",
	}, resp.DomainInfo)
	s.Equal(&types.DomainConfiguration{
		WorkflowExecutionRetentionPeriodInDays: retention,
		EmitMetric:                             true,
		HistoryArchivalStatus:                  types.ArchivalStatusDisabled.Ptr(),
		HistoryArchivalURI:                     "",
		VisibilityArchivalStatus:               types.ArchivalStatusDisabled.Ptr(),
		VisibilityArchivalURI:                  "",
		BadBinaries:                            &types.BadBinaries{Binaries: map[string]*types.BadBinaryInfo{}},
		IsolationGroups:                        &types.IsolationGroupConfiguration{},
		AsyncWorkflowConfig:                    &types.AsyncWorkflowConfiguration{},
	}, resp.Configuration)
	s.Equal(&types.DomainReplicationConfiguration{
		ActiveClusterName: s.ClusterMetadata.GetCurrentClusterName(),
		Clusters:          clusters,
	}, resp.ReplicationConfiguration)
	s.Equal(common.EmptyVersion, resp.GetFailoverVersion())
	s.Equal(isGlobalDomain, resp.GetIsGlobalDomain())
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestRegisterGetDomain_LocalDomain_NoDefault() {
	domainName := s.getRandomDomainName()
	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	activeClusterName := cluster.TestCurrentClusterName
	clusters := []*types.ClusterReplicationConfiguration{
		{
			ClusterName: activeClusterName,
		},
	}
	data := map[string]string{"some random key": "some random value"}
	isGlobalDomain := false

	var expectedClusters []*types.ClusterReplicationConfiguration
	for _, replicationConfig := range cluster.GetOrUseDefaultClusters(s.ClusterMetadata.GetCurrentClusterName(), nil) {
		expectedClusters = append(expectedClusters, &types.ClusterReplicationConfiguration{
			ClusterName: replicationConfig.ClusterName,
		})
	}

	err := s.handler.RegisterDomain(context.Background(), &types.RegisterDomainRequest{
		Name:                                   domainName,
		Description:                            description,
		OwnerEmail:                             email,
		WorkflowExecutionRetentionPeriodInDays: retention,
		EmitMetric:                             common.BoolPtr(emitMetric),
		Clusters:                               clusters,
		ActiveClusterName:                      activeClusterName,
		Data:                                   data,
		IsGlobalDomain:                         isGlobalDomain,
	})
	s.Nil(err)

	resp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.Nil(err)

	s.NotEmpty(resp.DomainInfo.GetUUID())
	resp.DomainInfo.UUID = ""
	s.Equal(&types.DomainInfo{
		Name:        domainName,
		Status:      types.DomainStatusRegistered.Ptr(),
		Description: description,
		OwnerEmail:  email,
		Data:        data,
		UUID:        "",
	}, resp.DomainInfo)
	s.Equal(&types.DomainConfiguration{
		WorkflowExecutionRetentionPeriodInDays: retention,
		EmitMetric:                             emitMetric,
		HistoryArchivalStatus:                  types.ArchivalStatusDisabled.Ptr(),
		HistoryArchivalURI:                     "",
		VisibilityArchivalStatus:               types.ArchivalStatusDisabled.Ptr(),
		VisibilityArchivalURI:                  "",
		BadBinaries:                            &types.BadBinaries{Binaries: map[string]*types.BadBinaryInfo{}},
		IsolationGroups:                        &types.IsolationGroupConfiguration{},
		AsyncWorkflowConfig:                    &types.AsyncWorkflowConfiguration{},
	}, resp.Configuration)
	s.Equal(&types.DomainReplicationConfiguration{
		ActiveClusterName: s.ClusterMetadata.GetCurrentClusterName(),
		Clusters:          expectedClusters,
	}, resp.ReplicationConfiguration)
	s.Equal(common.EmptyVersion, resp.GetFailoverVersion())
	s.Equal(isGlobalDomain, resp.GetIsGlobalDomain())
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestUpdateGetDomain_LocalDomain_NoAttrSet() {
	domainName := s.getRandomDomainName()
	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	data := map[string]string{"some random key": "some random value"}
	var clusters []*types.ClusterReplicationConfiguration
	for _, replicationConfig := range cluster.GetOrUseDefaultClusters(s.ClusterMetadata.GetCurrentClusterName(), nil) {
		clusters = append(clusters, &types.ClusterReplicationConfiguration{
			ClusterName: replicationConfig.ClusterName,
		})
	}
	isGlobalDomain := false

	err := s.handler.RegisterDomain(context.Background(), &types.RegisterDomainRequest{
		Name:                                   domainName,
		Description:                            description,
		OwnerEmail:                             email,
		WorkflowExecutionRetentionPeriodInDays: retention,
		EmitMetric:                             common.BoolPtr(emitMetric),
		Clusters:                               clusters,
		ActiveClusterName:                      s.ClusterMetadata.GetCurrentClusterName(),
		Data:                                   data,
		IsGlobalDomain:                         isGlobalDomain,
	})
	s.Nil(err)

	fnTest := func(info *types.DomainInfo, config *types.DomainConfiguration,
		replicationConfig *types.DomainReplicationConfiguration, isGlobalDomain bool, failoverVersion int64) {
		s.NotEmpty(info.GetUUID())
		info.UUID = ""
		s.Equal(&types.DomainInfo{
			Name:        domainName,
			Status:      types.DomainStatusRegistered.Ptr(),
			Description: description,
			OwnerEmail:  email,
			Data:        data,
			UUID:        "",
		}, info)
		s.Equal(&types.DomainConfiguration{
			WorkflowExecutionRetentionPeriodInDays: retention,
			EmitMetric:                             emitMetric,
			HistoryArchivalStatus:                  types.ArchivalStatusDisabled.Ptr(),
			HistoryArchivalURI:                     "",
			VisibilityArchivalStatus:               types.ArchivalStatusDisabled.Ptr(),
			VisibilityArchivalURI:                  "",
			BadBinaries:                            &types.BadBinaries{Binaries: map[string]*types.BadBinaryInfo{}},
			IsolationGroups:                        &types.IsolationGroupConfiguration{},
			AsyncWorkflowConfig:                    &types.AsyncWorkflowConfiguration{},
		}, config)
		s.Equal(&types.DomainReplicationConfiguration{
			ActiveClusterName: s.ClusterMetadata.GetCurrentClusterName(),
			Clusters:          clusters,
		}, replicationConfig)
		s.Equal(common.EmptyVersion, failoverVersion)
		s.Equal(isGlobalDomain, isGlobalDomain)
	}

	updateResp, err := s.handler.UpdateDomain(context.Background(), &types.UpdateDomainRequest{
		Name: domainName,
	})
	s.Nil(err)
	fnTest(
		updateResp.DomainInfo,
		updateResp.Configuration,
		updateResp.ReplicationConfiguration,
		updateResp.GetIsGlobalDomain(),
		updateResp.GetFailoverVersion(),
	)

	getResp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.Nil(err)
	fnTest(
		getResp.DomainInfo,
		getResp.Configuration,
		getResp.ReplicationConfiguration,
		getResp.GetIsGlobalDomain(),
		getResp.GetFailoverVersion(),
	)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestUpdateGetDomain_LocalDomain_AllAttrSet() {
	domainName := s.getRandomDomainName()
	isGlobalDomain := false
	err := s.handler.RegisterDomain(context.Background(), &types.RegisterDomainRequest{
		Name:                                   domainName,
		IsGlobalDomain:                         isGlobalDomain,
		WorkflowExecutionRetentionPeriodInDays: 1,
	})
	s.Nil(err)

	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	data := map[string]string{"some random key": "some random value"}
	var clusters []*types.ClusterReplicationConfiguration
	for _, replicationConfig := range cluster.GetOrUseDefaultClusters(s.ClusterMetadata.GetCurrentClusterName(), nil) {
		clusters = append(clusters, &types.ClusterReplicationConfiguration{
			ClusterName: replicationConfig.ClusterName,
		})
	}

	fnTest := func(info *types.DomainInfo, config *types.DomainConfiguration,
		replicationConfig *types.DomainReplicationConfiguration, isGlobalDomain bool, failoverVersion int64) {
		s.NotEmpty(info.GetUUID())
		info.UUID = ""
		s.Equal(&types.DomainInfo{
			Name:        domainName,
			Status:      types.DomainStatusRegistered.Ptr(),
			Description: description,
			OwnerEmail:  email,
			Data:        data,
			UUID:        "",
		}, info)
		s.Equal(&types.DomainConfiguration{
			WorkflowExecutionRetentionPeriodInDays: retention,
			EmitMetric:                             emitMetric,
			HistoryArchivalStatus:                  types.ArchivalStatusDisabled.Ptr(),
			HistoryArchivalURI:                     "",
			VisibilityArchivalStatus:               types.ArchivalStatusDisabled.Ptr(),
			VisibilityArchivalURI:                  "",
			BadBinaries:                            &types.BadBinaries{Binaries: map[string]*types.BadBinaryInfo{}},
			IsolationGroups:                        &types.IsolationGroupConfiguration{},
			AsyncWorkflowConfig:                    &types.AsyncWorkflowConfiguration{},
		}, config)
		s.Equal(&types.DomainReplicationConfiguration{
			ActiveClusterName: s.ClusterMetadata.GetCurrentClusterName(),
			Clusters:          clusters,
		}, replicationConfig)
		s.Equal(common.EmptyVersion, failoverVersion)
		s.Equal(isGlobalDomain, isGlobalDomain)
	}

	updateResp, err := s.handler.UpdateDomain(context.Background(), &types.UpdateDomainRequest{
		Name:                                   domainName,
		Description:                            common.StringPtr(description),
		OwnerEmail:                             common.StringPtr(email),
		Data:                                   data,
		WorkflowExecutionRetentionPeriodInDays: common.Int32Ptr(retention),
		EmitMetric:                             common.BoolPtr(emitMetric),
		HistoryArchivalStatus:                  types.ArchivalStatusDisabled.Ptr(),
		HistoryArchivalURI:                     common.StringPtr(""),
		VisibilityArchivalStatus:               types.ArchivalStatusDisabled.Ptr(),
		VisibilityArchivalURI:                  common.StringPtr(""),
		BadBinaries:                            &types.BadBinaries{Binaries: map[string]*types.BadBinaryInfo{}},
		ActiveClusterName:                      common.StringPtr(s.ClusterMetadata.GetCurrentClusterName()),
		Clusters:                               clusters,
	})
	s.Nil(err)
	fnTest(
		updateResp.DomainInfo,
		updateResp.Configuration,
		updateResp.ReplicationConfiguration,
		updateResp.GetIsGlobalDomain(),
		updateResp.GetFailoverVersion(),
	)

	getResp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.Nil(err)
	fnTest(
		getResp.DomainInfo,
		getResp.Configuration,
		getResp.ReplicationConfiguration,
		getResp.GetIsGlobalDomain(),
		getResp.GetFailoverVersion(),
	)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestDeprecateGetDomain_LocalDomain() {
	domainName := s.getRandomDomainName()
	domain := s.setupLocalDomain(domainName)

	err := s.handler.DeprecateDomain(context.Background(), &types.DeprecateDomainRequest{
		Name: domainName,
	})
	s.Nil(err)

	expectedResp := domain
	expectedResp.DomainInfo.Status = types.DomainStatusDeprecated.Ptr()

	getResp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.Nil(err)
	assertDomainEqual(s.Suite, getResp, expectedResp)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestRegisterGetDomain_GlobalDomain_AllDefault() {
	domainName := s.getRandomDomainName()
	isGlobalDomain := true
	var clusters []*types.ClusterReplicationConfiguration
	for _, replicationConfig := range cluster.GetOrUseDefaultClusters(s.ClusterMetadata.GetCurrentClusterName(), nil) {
		clusters = append(clusters, &types.ClusterReplicationConfiguration{
			ClusterName: replicationConfig.ClusterName,
		})
	}
	s.Equal(1, len(clusters))

	err := s.handler.RegisterDomain(context.Background(), &types.RegisterDomainRequest{
		Name:           domainName,
		IsGlobalDomain: isGlobalDomain,
	})
	s.IsType(&types.BadRequestError{}, err)

	resp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.IsType(&types.EntityNotExistsError{}, err)
	s.Nil(resp)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestRegisterGetDomain_GlobalDomain_NoDefault() {
	domainName := s.getRandomDomainName()
	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	activeClusterName := ""
	clusters := []*types.ClusterReplicationConfiguration{}
	for clusterName := range s.ClusterMetadata.GetEnabledClusterInfo() {
		if clusterName != s.ClusterMetadata.GetCurrentClusterName() {
			activeClusterName = clusterName
		}
		clusters = append(clusters, &types.ClusterReplicationConfiguration{
			ClusterName: clusterName,
		})
	}
	s.True(len(activeClusterName) > 0)
	s.True(len(clusters) > 1)
	data := map[string]string{"some random key": "some random value"}
	isGlobalDomain := true

	err := s.handler.RegisterDomain(context.Background(), &types.RegisterDomainRequest{
		Name:                                   domainName,
		Description:                            description,
		OwnerEmail:                             email,
		WorkflowExecutionRetentionPeriodInDays: retention,
		EmitMetric:                             common.BoolPtr(emitMetric),
		Clusters:                               clusters,
		ActiveClusterName:                      activeClusterName,
		Data:                                   data,
		IsGlobalDomain:                         isGlobalDomain,
	})
	s.IsType(&types.BadRequestError{}, err)

	resp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.IsType(&types.EntityNotExistsError{}, err)
	s.Nil(resp)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestUpdateGetDomain_GlobalDomain_NoAttrSet() {
	domainName := s.getRandomDomainName()
	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	activeClusterName := ""
	clusters := []*persistence.ClusterReplicationConfig{}
	for clusterName := range s.ClusterMetadata.GetEnabledClusterInfo() {
		if clusterName != s.ClusterMetadata.GetCurrentClusterName() {
			activeClusterName = clusterName
		}
		clusters = append(clusters, &persistence.ClusterReplicationConfig{
			ClusterName: clusterName,
		})
	}
	s.True(len(activeClusterName) > 0)
	s.True(len(clusters) > 1)
	data := map[string]string{"some random key": "some random value"}
	isGlobalDomain := true

	_, err := s.DomainManager.CreateDomain(context.Background(), &persistence.CreateDomainRequest{
		Info: &persistence.DomainInfo{
			ID:          uuid.New(),
			Name:        domainName,
			Status:      persistence.DomainStatusRegistered,
			Description: description,
			OwnerEmail:  email,
			Data:        data,
		},
		Config: &persistence.DomainConfig{
			Retention:                retention,
			EmitMetric:               emitMetric,
			HistoryArchivalStatus:    types.ArchivalStatusDisabled,
			HistoryArchivalURI:       "",
			VisibilityArchivalStatus: types.ArchivalStatusDisabled,
			VisibilityArchivalURI:    "",
		},
		ReplicationConfig: &persistence.DomainReplicationConfig{
			ActiveClusterName: activeClusterName,
			Clusters:          clusters,
		},
		IsGlobalDomain:  isGlobalDomain,
		ConfigVersion:   0,
		FailoverVersion: s.ClusterMetadata.GetNextFailoverVersion(activeClusterName, 0, "some-domain"),
	})
	s.Nil(err)

	resp, err := s.handler.UpdateDomain(context.Background(), &types.UpdateDomainRequest{
		Name: domainName,
	})
	s.IsType(&types.BadRequestError{}, err)
	s.Nil(resp)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestUpdateGetDomain_GlobalDomain_AllAttrSet() {
	domainName := s.getRandomDomainName()
	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	activeClusterName := ""
	clusters := []*types.ClusterReplicationConfiguration{}
	clustersDB := []*persistence.ClusterReplicationConfig{}
	for clusterName := range s.ClusterMetadata.GetEnabledClusterInfo() {
		if clusterName != s.ClusterMetadata.GetCurrentClusterName() {
			activeClusterName = clusterName
		}
		clusters = append(clusters, &types.ClusterReplicationConfiguration{
			ClusterName: clusterName,
		})
		clustersDB = append(clustersDB, &persistence.ClusterReplicationConfig{
			ClusterName: clusterName,
		})
	}
	s.True(len(activeClusterName) > 0)
	s.True(len(clusters) > 1)
	s.True(len(clustersDB) > 1)
	data := map[string]string{"some random key": "some random value"}
	isGlobalDomain := true

	_, err := s.DomainManager.CreateDomain(context.Background(), &persistence.CreateDomainRequest{
		Info: &persistence.DomainInfo{
			ID:          uuid.New(),
			Name:        domainName,
			Status:      persistence.DomainStatusRegistered,
			Description: "",
			OwnerEmail:  "",
			Data:        map[string]string{},
		},
		Config: &persistence.DomainConfig{
			Retention:                0,
			EmitMetric:               false,
			HistoryArchivalStatus:    types.ArchivalStatusDisabled,
			HistoryArchivalURI:       "",
			VisibilityArchivalStatus: types.ArchivalStatusDisabled,
			VisibilityArchivalURI:    "",
		},
		ReplicationConfig: &persistence.DomainReplicationConfig{
			ActiveClusterName: activeClusterName,
			Clusters:          clustersDB,
		},
		IsGlobalDomain:  isGlobalDomain,
		ConfigVersion:   0,
		FailoverVersion: s.ClusterMetadata.GetNextFailoverVersion(activeClusterName, 0, "some-domain"),
	})
	s.Nil(err)

	updateResp, err := s.handler.UpdateDomain(context.Background(), &types.UpdateDomainRequest{
		Name:                                   domainName,
		Description:                            common.StringPtr(description),
		OwnerEmail:                             common.StringPtr(email),
		Data:                                   data,
		WorkflowExecutionRetentionPeriodInDays: common.Int32Ptr(retention),
		EmitMetric:                             common.BoolPtr(emitMetric),
		HistoryArchivalStatus:                  types.ArchivalStatusDisabled.Ptr(),
		HistoryArchivalURI:                     common.StringPtr(""),
		VisibilityArchivalStatus:               types.ArchivalStatusDisabled.Ptr(),
		VisibilityArchivalURI:                  common.StringPtr(""),
		BadBinaries:                            &types.BadBinaries{Binaries: map[string]*types.BadBinaryInfo{}},
		ActiveClusterName:                      nil,
		Clusters:                               clusters,
	})
	s.IsType(&types.BadRequestError{}, err)
	s.Nil(updateResp)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestUpdateGetDomain_GlobalDomain_Failover() {
	domainName := s.getRandomDomainName()
	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	prevActiveClusterName := ""
	nextActiveClusterName := s.ClusterMetadata.GetCurrentClusterName()
	clusters := []*types.ClusterReplicationConfiguration{}
	clustersDB := []*persistence.ClusterReplicationConfig{}
	for clusterName := range s.ClusterMetadata.GetEnabledClusterInfo() {
		if clusterName != s.ClusterMetadata.GetCurrentClusterName() {
			prevActiveClusterName = clusterName
		}
		clusters = append(clusters, &types.ClusterReplicationConfiguration{
			ClusterName: clusterName,
		})
		clustersDB = append(clustersDB, &persistence.ClusterReplicationConfig{
			ClusterName: clusterName,
		})
	}
	s.True(len(prevActiveClusterName) > 0)
	s.True(len(clusters) > 1)
	s.True(len(clustersDB) > 1)
	data := map[string]string{"some random key": "some random value"}
	isGlobalDomain := true

	_, err := s.DomainManager.CreateDomain(context.Background(), &persistence.CreateDomainRequest{
		Info: &persistence.DomainInfo{
			ID:          uuid.New(),
			Name:        domainName,
			Status:      persistence.DomainStatusRegistered,
			Description: description,
			OwnerEmail:  email,
			Data:        data,
		},
		Config: &persistence.DomainConfig{
			Retention:                retention,
			EmitMetric:               emitMetric,
			HistoryArchivalStatus:    types.ArchivalStatusDisabled,
			HistoryArchivalURI:       "",
			VisibilityArchivalStatus: types.ArchivalStatusDisabled,
			VisibilityArchivalURI:    "",
		},
		ReplicationConfig: &persistence.DomainReplicationConfig{
			ActiveClusterName: prevActiveClusterName,
			Clusters:          clustersDB,
		},
		IsGlobalDomain:  isGlobalDomain,
		ConfigVersion:   0,
		FailoverVersion: s.ClusterMetadata.GetNextFailoverVersion(prevActiveClusterName, 0, "some-domain"),
	})
	s.Nil(err)

	fnTest := func(info *types.DomainInfo, config *types.DomainConfiguration,
		replicationConfig *types.DomainReplicationConfiguration, isGlobalDomain bool, failoverVersion int64) {
		s.NotEmpty(info.GetUUID())
		info.UUID = ""
		s.Equal(&types.DomainInfo{
			Name:        domainName,
			Status:      types.DomainStatusRegistered.Ptr(),
			Description: description,
			OwnerEmail:  email,
			Data:        data,
			UUID:        "",
		}, info)
		s.Equal(&types.DomainConfiguration{
			WorkflowExecutionRetentionPeriodInDays: retention,
			EmitMetric:                             emitMetric,
			HistoryArchivalStatus:                  types.ArchivalStatusDisabled.Ptr(),
			HistoryArchivalURI:                     "",
			VisibilityArchivalStatus:               types.ArchivalStatusDisabled.Ptr(),
			VisibilityArchivalURI:                  "",
			BadBinaries:                            &types.BadBinaries{Binaries: map[string]*types.BadBinaryInfo{}},
			IsolationGroups:                        &types.IsolationGroupConfiguration{},
			AsyncWorkflowConfig:                    &types.AsyncWorkflowConfiguration{},
		}, config)
		s.Equal(&types.DomainReplicationConfiguration{
			ActiveClusterName: nextActiveClusterName,
			Clusters:          clusters,
		}, replicationConfig)
		s.Equal(s.ClusterMetadata.GetNextFailoverVersion(
			nextActiveClusterName,
			s.ClusterMetadata.GetNextFailoverVersion(prevActiveClusterName, 0, "some-domain"),
			"some-domain"), failoverVersion)
		s.Equal(isGlobalDomain, isGlobalDomain)
	}

	s.mockProducer.On("Publish", mock.Anything, mock.Anything).Return(nil).Once()

	updateResp, err := s.handler.UpdateDomain(context.Background(), &types.UpdateDomainRequest{
		Name:              domainName,
		ActiveClusterName: common.StringPtr(s.ClusterMetadata.GetCurrentClusterName()),
	})
	s.Nil(err)
	fnTest(
		updateResp.DomainInfo,
		updateResp.Configuration,
		updateResp.ReplicationConfiguration,
		updateResp.GetIsGlobalDomain(),
		updateResp.GetFailoverVersion(),
	)

	getResp, err := s.handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.Nil(err)
	fnTest(
		getResp.DomainInfo,
		getResp.Configuration,
		getResp.ReplicationConfiguration,
		getResp.GetIsGlobalDomain(),
		getResp.GetFailoverVersion(),
	)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) TestDeprecateGetDomain_GlobalDomain() {
	domainName := s.getRandomDomainName()
	s.setupGlobalDomainWithMetadataManager(domainName)

	err := s.handler.DeprecateDomain(context.Background(), &types.DeprecateDomainRequest{
		Name: domainName,
	})
	s.IsType(&types.BadRequestError{}, err)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) getRandomDomainName() string {
	return "domain" + uuid.New()
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) setupLocalDomain(domainName string) *types.DescribeDomainResponse {
	return setupLocalDomain(s.Suite, s.handler, s.ClusterMetadata, domainName)
}

func (s *domainHandlerGlobalDomainEnabledNotPrimaryClusterSuite) setupGlobalDomainWithMetadataManager(domainName string) *types.DescribeDomainResponse {
	return setupGlobalDomainWithMetadataManager(s.Suite, s.handler, s.ClusterMetadata, s.DomainManager, domainName)
}

func setupGlobalDomainWithMetadataManager(s suite.Suite, handler *handlerImpl, clusterMetadata cluster.Metadata, domainManager persistence.DomainManager, domainName string) *types.DescribeDomainResponse {
	description := "some random description"
	email := "some random email"
	retention := int32(7)
	emitMetric := true
	activeClusterName := ""
	clusters := []*persistence.ClusterReplicationConfig{}
	for clusterName := range clusterMetadata.GetAllClusterInfo() {
		if clusterName != clusterMetadata.GetCurrentClusterName() {
			activeClusterName = clusterName
		}
		clusters = append(clusters, &persistence.ClusterReplicationConfig{
			ClusterName: clusterName,
		})
	}
	s.True(len(activeClusterName) > 0)
	s.True(len(clusters) > 1)
	data := map[string]string{"some random key": "some random value"}
	isGlobalDomain := true

	_, err := domainManager.CreateDomain(context.Background(), &persistence.CreateDomainRequest{
		Info: &persistence.DomainInfo{
			ID:          uuid.New(),
			Name:        domainName,
			Status:      persistence.DomainStatusRegistered,
			Description: description,
			OwnerEmail:  email,
			Data:        data,
		},
		Config: &persistence.DomainConfig{
			Retention:                retention,
			EmitMetric:               emitMetric,
			HistoryArchivalStatus:    types.ArchivalStatusDisabled,
			HistoryArchivalURI:       "",
			VisibilityArchivalStatus: types.ArchivalStatusDisabled,
			VisibilityArchivalURI:    "",
		},
		ReplicationConfig: &persistence.DomainReplicationConfig{
			ActiveClusterName: activeClusterName,
			Clusters:          clusters,
		},
		IsGlobalDomain:  isGlobalDomain,
		ConfigVersion:   0,
		FailoverVersion: clusterMetadata.GetNextFailoverVersion(activeClusterName, 0, "some-domain"),
	})
	s.Nil(err)

	getResp, err := handler.DescribeDomain(context.Background(), &types.DescribeDomainRequest{
		Name: common.StringPtr(domainName),
	})
	s.Nil(err)
	return getResp
}
