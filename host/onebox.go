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

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pborman/uuid"
	"github.com/uber-go/tally"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	cwsc "go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
	"go.uber.org/cadence/compatibility"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/yarpc/transport/tchannel"

	adminClient "github.com/uber/cadence/client/admin"
	frontendClient "github.com/uber/cadence/client/frontend"
	historyClient "github.com/uber/cadence/client/history"
	"github.com/uber/cadence/common"
	carchiver "github.com/uber/cadence/common/archiver"
	"github.com/uber/cadence/common/archiver/provider"
	"github.com/uber/cadence/common/authorization"
	"github.com/uber/cadence/common/cache"
	cc "github.com/uber/cadence/common/client"
	"github.com/uber/cadence/common/cluster"
	"github.com/uber/cadence/common/config"
	"github.com/uber/cadence/common/domain"
	"github.com/uber/cadence/common/dynamicconfig"
	"github.com/uber/cadence/common/elasticsearch"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/log/tag"
	"github.com/uber/cadence/common/membership"
	"github.com/uber/cadence/common/messaging"
	"github.com/uber/cadence/common/metrics"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/pinot"
	"github.com/uber/cadence/common/resource"
	"github.com/uber/cadence/common/rpc"
	"github.com/uber/cadence/common/service"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/frontend"
	"github.com/uber/cadence/service/history"
	"github.com/uber/cadence/service/matching"
	"github.com/uber/cadence/service/worker"
	"github.com/uber/cadence/service/worker/archiver"
	"github.com/uber/cadence/service/worker/indexer"
	"github.com/uber/cadence/service/worker/replicator"
)

// Cadence hosts all of cadence services in one process
type Cadence interface {
	Start() error
	Stop()
	GetAdminClient() adminClient.Client
	GetFrontendClient() frontendClient.Client
	FrontendHost() membership.HostInfo
	GetHistoryClient() historyClient.Client
	GetExecutionManagerFactory() persistence.ExecutionManagerFactory
}

type (
	cadenceImpl struct {
		frontendService common.Daemon
		matchingService common.Daemon
		historyServices []common.Daemon

		adminClient                   adminClient.Client
		frontendClient                frontendClient.Client
		historyClient                 historyClient.Client
		logger                        log.Logger
		clusterMetadata               cluster.Metadata
		persistenceConfig             config.Persistence
		messagingClient               messaging.Client
		domainManager                 persistence.DomainManager
		historyV2Mgr                  persistence.HistoryManager
		executionMgrFactory           persistence.ExecutionManagerFactory
		domainReplicationQueue        domain.ReplicationQueue
		shutdownCh                    chan struct{}
		shutdownWG                    sync.WaitGroup
		clusterNo                     int // cluster number
		replicator                    *replicator.Replicator
		clientWorker                  archiver.ClientWorker
		indexer                       *indexer.Indexer
		archiverMetadata              carchiver.ArchivalMetadata
		archiverProvider              provider.ArchiverProvider
		historyConfig                 *HistoryConfig
		esConfig                      *config.ElasticSearchConfig
		esClient                      elasticsearch.GenericClient
		workerConfig                  *WorkerConfig
		mockAdminClient               map[string]adminClient.Client
		domainReplicationTaskExecutor domain.ReplicationTaskExecutor
		authorizationConfig           config.Authorization
		pinotConfig                   *config.PinotVisibilityConfig
		pinotClient                   pinot.GenericClient
	}

	// HistoryConfig contains configs for history service
	HistoryConfig struct {
		NumHistoryShards       int
		NumHistoryHosts        int
		HistoryCountLimitError int
		HistoryCountLimitWarn  int
	}

	// CadenceParams contains everything needed to bootstrap Cadence
	CadenceParams struct {
		ClusterMetadata               cluster.Metadata
		PersistenceConfig             config.Persistence
		MessagingClient               messaging.Client
		DomainManager                 persistence.DomainManager
		HistoryV2Mgr                  persistence.HistoryManager
		ExecutionMgrFactory           persistence.ExecutionManagerFactory
		DomainReplicationQueue        domain.ReplicationQueue
		Logger                        log.Logger
		ClusterNo                     int
		ArchiverMetadata              carchiver.ArchivalMetadata
		ArchiverProvider              provider.ArchiverProvider
		EnableReadHistoryFromArchival bool
		HistoryConfig                 *HistoryConfig
		ESConfig                      *config.ElasticSearchConfig
		ESClient                      elasticsearch.GenericClient
		WorkerConfig                  *WorkerConfig
		MockAdminClient               map[string]adminClient.Client
		DomainReplicationTaskExecutor domain.ReplicationTaskExecutor
		AuthorizationConfig           config.Authorization
		PinotConfig                   *config.PinotVisibilityConfig
		PinotClient                   pinot.GenericClient
	}
)

// NewCadence returns an instance that hosts full cadence in one process
func NewCadence(params *CadenceParams) Cadence {
	return &cadenceImpl{
		logger:                        params.Logger,
		clusterMetadata:               params.ClusterMetadata,
		persistenceConfig:             params.PersistenceConfig,
		messagingClient:               params.MessagingClient,
		domainManager:                 params.DomainManager,
		historyV2Mgr:                  params.HistoryV2Mgr,
		executionMgrFactory:           params.ExecutionMgrFactory,
		domainReplicationQueue:        params.DomainReplicationQueue,
		shutdownCh:                    make(chan struct{}),
		clusterNo:                     params.ClusterNo,
		esConfig:                      params.ESConfig,
		esClient:                      params.ESClient,
		archiverMetadata:              params.ArchiverMetadata,
		archiverProvider:              params.ArchiverProvider,
		historyConfig:                 params.HistoryConfig,
		workerConfig:                  params.WorkerConfig,
		mockAdminClient:               params.MockAdminClient,
		domainReplicationTaskExecutor: params.DomainReplicationTaskExecutor,
		authorizationConfig:           params.AuthorizationConfig,
		pinotConfig:                   params.PinotConfig,
		pinotClient:                   params.PinotClient,
	}
}

func (c *cadenceImpl) enableWorker() bool {
	return c.workerConfig.EnableArchiver || c.workerConfig.EnableIndexer || c.workerConfig.EnableReplicator
}

func (c *cadenceImpl) Start() error {
	hosts := make(map[string][]membership.HostInfo)
	hosts[service.Frontend] = []membership.HostInfo{c.FrontendHost()}
	hosts[service.Matching] = []membership.HostInfo{c.MatchingServiceHost()}
	hosts[service.History] = c.HistoryHosts()
	if c.enableWorker() {
		hosts[service.Worker] = []membership.HostInfo{c.WorkerServiceHost()}
	}

	// create cadence-system domain, this must be created before starting
	// the services - so directly use the metadataManager to create this
	if err := c.createSystemDomain(); err != nil {
		return err
	}

	var startWG sync.WaitGroup
	startWG.Add(2)
	go c.startHistory(hosts, &startWG)
	go c.startMatching(hosts, &startWG)
	startWG.Wait()

	startWG.Add(1)
	go c.startFrontend(hosts, &startWG)
	startWG.Wait()

	if c.enableWorker() {
		startWG.Add(1)
		go c.startWorker(hosts, &startWG)
		startWG.Wait()
	}

	return nil
}

func (c *cadenceImpl) Stop() {
	if c.enableWorker() {
		c.shutdownWG.Add(4)
	} else {
		c.shutdownWG.Add(3)
	}
	c.frontendService.Stop()
	for _, historyService := range c.historyServices {
		historyService.Stop()
	}
	c.matchingService.Stop()
	if c.workerConfig.EnableReplicator {
		c.replicator.Stop()
	}
	if c.workerConfig.EnableArchiver {
		c.clientWorker.Stop()
	}
	close(c.shutdownCh)
	c.shutdownWG.Wait()
}

func newHost(tchan uint16) membership.HostInfo {
	address := "127.0.0.1"
	return membership.NewDetailedHostInfo(
		fmt.Sprintf("%s:%d", address, tchan),
		fmt.Sprintf("%s_%d", address, tchan),
		membership.PortMap{
			membership.PortTchannel: tchan,
			membership.PortGRPC:     tchan + 10,
		},
	)

}

func (c *cadenceImpl) FrontendHost() membership.HostInfo {
	var tchan uint16
	switch c.clusterNo {
	case 0:
		tchan = 7104
	case 1:
		tchan = 8104
	case 2:
		tchan = 9104
	case 3:
		tchan = 10104
	default:
		tchan = 7104
	}

	return newHost(tchan)

}

func (c *cadenceImpl) FrontendPProfPort() int {
	switch c.clusterNo {
	case 0:
		return 7105
	case 1:
		return 8105
	case 2:
		return 9105
	case 3:
		return 10105
	default:
		return 7105
	}
}

func (c *cadenceImpl) HistoryHosts() []membership.HostInfo {
	var hosts []membership.HostInfo
	var startPort int
	switch c.clusterNo {
	case 0:
		startPort = 7201
	case 1:
		startPort = 8201
	case 2:
		startPort = 9201
	case 3:
		startPort = 10201
	default:
		startPort = 7201
	}
	for i := 0; i < c.historyConfig.NumHistoryHosts; i++ {
		port := startPort + i
		hosts = append(hosts, newHost(uint16(port)))
	}

	c.logger.Info("History hosts", tag.Value(hosts))
	return hosts
}

func (c *cadenceImpl) HistoryPProfPort() []int {
	var ports []int
	var startPort int
	switch c.clusterNo {
	case 0:
		startPort = 7301
	case 1:
		startPort = 8301
	case 2:
		startPort = 9301
	case 3:
		startPort = 10301
	default:
		startPort = 7301
	}
	for i := 0; i < c.historyConfig.NumHistoryHosts; i++ {
		port := startPort + i
		ports = append(ports, port)
	}

	c.logger.Info("History pprof ports", tag.Value(ports))
	return ports
}

func (c *cadenceImpl) MatchingServiceHost() membership.HostInfo {
	var tchan uint16
	switch c.clusterNo {
	case 0:
		tchan = 7106
	case 1:
		tchan = 8106
	case 2:
		tchan = 9106
	case 3:
		tchan = 10106
	default:
		tchan = 7106
	}

	return newHost(tchan)

}

func (c *cadenceImpl) MatchingPProfPort() int {
	switch c.clusterNo {
	case 0:
		return 7107
	case 1:
		return 8107
	case 2:
		return 9107
	case 3:
		return 10107
	default:
		return 7107
	}
}

func (c *cadenceImpl) WorkerServiceHost() membership.HostInfo {
	var tchan uint16
	switch c.clusterNo {
	case 0:
		tchan = 7108
	case 1:
		tchan = 8108
	case 2:
		tchan = 9108
	case 3:
		tchan = 10108
	default:
		tchan = 7108
	}
	return newHost(tchan)
}

func (c *cadenceImpl) WorkerPProfPort() int {
	switch c.clusterNo {
	case 0:
		return 7109
	case 1:
		return 8109
	case 2:
		return 9109
	case 3:
		return 10109
	default:
		return 7109
	}
}

func (c *cadenceImpl) GetAdminClient() adminClient.Client {
	return c.adminClient
}

func (c *cadenceImpl) GetFrontendClient() frontendClient.Client {
	return c.frontendClient
}

func (c *cadenceImpl) GetHistoryClient() historyClient.Client {
	return c.historyClient
}

func (c *cadenceImpl) startFrontend(hosts map[string][]membership.HostInfo, startWG *sync.WaitGroup) {
	params := new(resource.Params)
	params.ClusterRedirectionPolicy = &config.ClusterRedirectionPolicy{}
	params.Name = service.Frontend
	params.Logger = c.logger
	params.ThrottledLogger = c.logger
	params.PProfInitializer = newPProfInitializerImpl(c.logger, c.FrontendPProfPort())
	params.RPCFactory = c.newRPCFactory(service.Frontend, c.FrontendHost())
	params.MetricScope = tally.NewTestScope(service.Frontend, make(map[string]string))
	params.MembershipResolver = newMembershipResolver(params.Name, hosts)
	params.ClusterMetadata = c.clusterMetadata
	params.MessagingClient = c.messagingClient
	params.MetricsClient = metrics.NewClient(params.MetricScope, service.GetMetricsServiceIdx(params.Name, c.logger))
	params.DynamicConfig = newIntegrationConfigClient(dynamicconfig.NewNopClient())
	params.ArchivalMetadata = c.archiverMetadata
	params.ArchiverProvider = c.archiverProvider
	params.ESConfig = c.esConfig
	params.ESClient = c.esClient
	params.PinotConfig = c.pinotConfig
	params.PinotClient = c.pinotClient
	var err error
	authorizer, err := authorization.NewAuthorizer(c.authorizationConfig, params.Logger, nil)
	if err != nil {
		c.logger.Fatal("Unable to create authorizer", tag.Error(err))
	}
	params.Authorizer = authorizer
	params.PersistenceConfig, err = copyPersistenceConfig(c.persistenceConfig)
	if err != nil {
		c.logger.Fatal("Failed to copy persistence config for frontend", tag.Error(err))
	}

	if c.pinotConfig != nil {
		pinotDataStoreName := "pinot-visibility"
		params.PersistenceConfig.AdvancedVisibilityStore = pinotDataStoreName
		params.DynamicConfig.UpdateValue(dynamicconfig.EnableReadVisibilityFromES, false)
		params.PersistenceConfig.DataStores[pinotDataStoreName] = config.DataStore{
			Pinot: c.pinotConfig,
		}
	} else if c.esConfig != nil {
		esDataStoreName := "es-visibility"
		params.PersistenceConfig.AdvancedVisibilityStore = esDataStoreName
		params.PersistenceConfig.DataStores[esDataStoreName] = config.DataStore{
			ElasticSearch: c.esConfig,
		}
	}

	frontendService, err := frontend.NewService(params)
	if err != nil {
		params.Logger.Fatal("unable to start frontend service", tag.Error(err))
	}

	if c.mockAdminClient != nil {
		clientBean := frontendService.GetClientBean()
		if clientBean != nil {
			for serviceName, client := range c.mockAdminClient {
				clientBean.SetRemoteAdminClient(serviceName, client)
			}
		}
	}

	c.frontendService = frontendService
	c.frontendClient = NewFrontendClient(frontendService.GetDispatcher())
	c.adminClient = NewAdminClient(frontendService.GetDispatcher())
	go frontendService.Start()

	startWG.Done()
	<-c.shutdownCh
	c.shutdownWG.Done()
}

func (c *cadenceImpl) startHistory(
	hosts map[string][]membership.HostInfo,
	startWG *sync.WaitGroup,
) {
	pprofPorts := c.HistoryPProfPort()
	for i, hostport := range c.HistoryHosts() {
		params := new(resource.Params)
		params.Name = service.History
		params.Logger = c.logger
		params.ThrottledLogger = c.logger
		params.PProfInitializer = newPProfInitializerImpl(c.logger, pprofPorts[i])
		params.RPCFactory = c.newRPCFactory(service.History, hostport)
		params.MetricScope = tally.NewTestScope(service.History, make(map[string]string))
		params.MembershipResolver = newMembershipResolver(params.Name, hosts)
		params.ClusterMetadata = c.clusterMetadata
		params.MessagingClient = c.messagingClient
		params.MetricsClient = metrics.NewClient(params.MetricScope, service.GetMetricsServiceIdx(params.Name, c.logger))
		integrationClient := newIntegrationConfigClient(dynamicconfig.NewNopClient())
		c.overrideHistoryDynamicConfig(integrationClient)
		params.DynamicConfig = integrationClient
		params.PublicClient = newPublicClient(params.RPCFactory.GetDispatcher())
		params.ArchivalMetadata = c.archiverMetadata
		params.ArchiverProvider = c.archiverProvider
		params.ESConfig = c.esConfig
		params.ESClient = c.esClient

		var err error
		params.PersistenceConfig, err = copyPersistenceConfig(c.persistenceConfig)
		if err != nil {
			c.logger.Fatal("Failed to copy persistence config for history", tag.Error(err))
		}

		if c.pinotConfig != nil {
			pinotDataStoreName := "pinot-visibility"
			params.PersistenceConfig.AdvancedVisibilityStore = pinotDataStoreName
			params.PersistenceConfig.DataStores[pinotDataStoreName] = config.DataStore{
				Pinot:         c.pinotConfig,
				ElasticSearch: c.esConfig,
			}
		} else if c.esConfig != nil {
			esDataStoreName := "es-visibility"
			params.PersistenceConfig.AdvancedVisibilityStore = esDataStoreName
			params.PersistenceConfig.DataStores[esDataStoreName] = config.DataStore{
				ElasticSearch: c.esConfig,
			}
		}

		historyService, err := history.NewService(params)
		if err != nil {
			params.Logger.Fatal("unable to start history service", tag.Error(err))
		}

		if c.mockAdminClient != nil {
			clientBean := historyService.GetClientBean()
			if clientBean != nil {
				for serviceName, client := range c.mockAdminClient {
					clientBean.SetRemoteAdminClient(serviceName, client)
				}
			}
		}

		// TODO: this is not correct when there are multiple history hosts as later client will overwrite previous ones.
		// However current interface for getting history client doesn't specify which client it needs and the tests that use this API
		// depends on the fact that there's only one history host.
		// Need to change those tests and modify the interface for getting history client.
		c.historyClient = NewHistoryClient(historyService.GetDispatcher())
		c.historyServices = append(c.historyServices, historyService)

		go historyService.Start()
	}

	startWG.Done()
	<-c.shutdownCh
	c.shutdownWG.Done()
}

func (c *cadenceImpl) startMatching(hosts map[string][]membership.HostInfo, startWG *sync.WaitGroup) {

	params := new(resource.Params)
	params.Name = service.Matching
	params.Logger = c.logger
	params.ThrottledLogger = c.logger
	params.PProfInitializer = newPProfInitializerImpl(c.logger, c.MatchingPProfPort())
	params.RPCFactory = c.newRPCFactory(service.Matching, c.MatchingServiceHost())
	params.MetricScope = tally.NewTestScope(service.Matching, make(map[string]string))
	params.MembershipResolver = newMembershipResolver(params.Name, hosts)
	params.ClusterMetadata = c.clusterMetadata
	params.MetricsClient = metrics.NewClient(params.MetricScope, service.GetMetricsServiceIdx(params.Name, c.logger))
	params.DynamicConfig = newIntegrationConfigClient(dynamicconfig.NewNopClient())
	params.ArchivalMetadata = c.archiverMetadata
	params.ArchiverProvider = c.archiverProvider

	var err error
	params.PersistenceConfig, err = copyPersistenceConfig(c.persistenceConfig)
	if err != nil {
		c.logger.Fatal("Failed to copy persistence config for matching", tag.Error(err))
	}

	matchingService, err := matching.NewService(params)
	if err != nil {
		params.Logger.Fatal("unable to start matching service", tag.Error(err))
	}
	if c.mockAdminClient != nil {
		clientBean := matchingService.GetClientBean()
		if clientBean != nil {
			for serviceName, client := range c.mockAdminClient {
				clientBean.SetRemoteAdminClient(serviceName, client)
			}
		}
	}
	c.matchingService = matchingService
	go c.matchingService.Start()

	startWG.Done()
	<-c.shutdownCh
	c.shutdownWG.Done()
}

func (c *cadenceImpl) startWorker(hosts map[string][]membership.HostInfo, startWG *sync.WaitGroup) {
	params := new(resource.Params)
	params.Name = service.Worker
	params.Logger = c.logger
	params.ThrottledLogger = c.logger
	params.PProfInitializer = newPProfInitializerImpl(c.logger, c.WorkerPProfPort())
	params.RPCFactory = c.newRPCFactory(service.Worker, c.WorkerServiceHost())
	params.MetricScope = tally.NewTestScope(service.Worker, make(map[string]string))
	params.MembershipResolver = newMembershipResolver(params.Name, hosts)
	params.ClusterMetadata = c.clusterMetadata
	params.MetricsClient = metrics.NewClient(params.MetricScope, service.GetMetricsServiceIdx(params.Name, c.logger))
	params.DynamicConfig = newIntegrationConfigClient(dynamicconfig.NewNopClient())
	params.ArchivalMetadata = c.archiverMetadata
	params.ArchiverProvider = c.archiverProvider

	var err error
	params.PersistenceConfig, err = copyPersistenceConfig(c.persistenceConfig)
	if err != nil {
		c.logger.Fatal("Failed to copy persistence config for worker", tag.Error(err))
	}
	params.PublicClient = newPublicClient(params.RPCFactory.GetDispatcher())
	service := NewService(params)
	service.Start()

	var replicatorDomainCache cache.DomainCache
	if c.workerConfig.EnableReplicator {
		metadataManager := persistence.NewDomainPersistenceMetricsClient(c.domainManager, service.GetMetricsClient(), c.logger, &c.persistenceConfig)
		replicatorDomainCache = cache.NewDomainCache(metadataManager, c.clusterMetadata, service.GetMetricsClient(), service.GetLogger())
		replicatorDomainCache.Start()
		c.startWorkerReplicator(service)
	}

	var clientWorkerDomainCache cache.DomainCache
	if c.workerConfig.EnableArchiver {
		metadataProxyManager := persistence.NewDomainPersistenceMetricsClient(c.domainManager, service.GetMetricsClient(), c.logger, &c.persistenceConfig)
		clientWorkerDomainCache = cache.NewDomainCache(metadataProxyManager, c.clusterMetadata, service.GetMetricsClient(), service.GetLogger())
		clientWorkerDomainCache.Start()
		c.startWorkerClientWorker(params, service, clientWorkerDomainCache)
	}

	if c.workerConfig.EnableIndexer {
		c.startWorkerIndexer(params, service)
	}

	startWG.Done()
	<-c.shutdownCh
	if c.workerConfig.EnableReplicator {
		replicatorDomainCache.Stop()
	}
	if c.workerConfig.EnableArchiver {
		clientWorkerDomainCache.Stop()
	}
	c.shutdownWG.Done()
}

func (c *cadenceImpl) startWorkerReplicator(svc Service) {
	c.replicator = replicator.NewReplicator(
		c.clusterMetadata,
		svc.GetClientBean(),
		c.logger,
		svc.GetMetricsClient(),
		svc.GetHostInfo(),
		svc.GetMembershipResolver(),
		c.domainReplicationQueue,
		c.domainReplicationTaskExecutor,
		time.Millisecond,
	)
	if err := c.replicator.Start(); err != nil {
		c.replicator.Stop()
		c.logger.Fatal("Fail to start replicator when start worker", tag.Error(err))
	}
}

func (c *cadenceImpl) startWorkerClientWorker(params *resource.Params, svc Service, domainCache cache.DomainCache) {
	workerConfig := worker.NewConfig(params)
	workerConfig.ArchiverConfig.ArchiverConcurrency = dynamicconfig.GetIntPropertyFn(10)
	historyArchiverBootstrapContainer := &carchiver.HistoryBootstrapContainer{
		HistoryV2Manager: c.historyV2Mgr,
		Logger:           c.logger,
		MetricsClient:    svc.GetMetricsClient(),
		ClusterMetadata:  c.clusterMetadata,
		DomainCache:      domainCache,
	}
	err := c.archiverProvider.RegisterBootstrapContainer(service.Worker, historyArchiverBootstrapContainer, &carchiver.VisibilityBootstrapContainer{})
	if err != nil {
		c.logger.Fatal("Failed to register archiver bootstrap container for worker service", tag.Error(err))
	}

	bc := &archiver.BootstrapContainer{
		PublicClient:     params.PublicClient,
		MetricsClient:    svc.GetMetricsClient(),
		Logger:           c.logger,
		HistoryV2Manager: c.historyV2Mgr,
		DomainCache:      domainCache,
		Config:           workerConfig.ArchiverConfig,
		ArchiverProvider: c.archiverProvider,
	}
	c.clientWorker = archiver.NewClientWorker(bc)
	if err := c.clientWorker.Start(); err != nil {
		c.clientWorker.Stop()
		c.logger.Fatal("Fail to start archiver when start worker", tag.Error(err))
	}
}

func (c *cadenceImpl) startWorkerIndexer(params *resource.Params, service Service) {
	params.DynamicConfig.UpdateValue(dynamicconfig.AdvancedVisibilityWritingMode, common.AdvancedVisibilityWritingModeDual)
	workerConfig := worker.NewConfig(params)
	c.indexer = indexer.NewIndexer(
		workerConfig.IndexerCfg,
		c.messagingClient,
		c.esClient,
		c.esConfig.Indices[common.VisibilityAppName],
		c.logger,
		service.GetMetricsClient())
	if err := c.indexer.Start(); err != nil {
		c.indexer.Stop()
		c.logger.Fatal("Fail to start indexer when start worker", tag.Error(err))
	}
}

func (c *cadenceImpl) createSystemDomain() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestPersistenceTimeout)
	defer cancel()

	_, err := c.domainManager.CreateDomain(ctx, &persistence.CreateDomainRequest{
		Info: &persistence.DomainInfo{
			ID:          uuid.New(),
			Name:        "cadence-system",
			Status:      persistence.DomainStatusRegistered,
			Description: "Cadence system domain",
		},
		Config: &persistence.DomainConfig{
			Retention:                1,
			HistoryArchivalStatus:    types.ArchivalStatusDisabled,
			VisibilityArchivalStatus: types.ArchivalStatusDisabled,
		},
		ReplicationConfig: &persistence.DomainReplicationConfig{},
		FailoverVersion:   common.EmptyVersion,
	})
	if err != nil {
		if _, ok := err.(*types.DomainAlreadyExistsError); ok {
			return nil
		}
		return fmt.Errorf("failed to create cadence-system domain: %v", err)
	}
	return nil
}

func (c *cadenceImpl) GetExecutionManagerFactory() persistence.ExecutionManagerFactory {
	return c.executionMgrFactory
}

func (c *cadenceImpl) overrideHistoryDynamicConfig(client *dynamicClient) {
	client.OverrideValue(dynamicconfig.HistoryMgrNumConns, c.historyConfig.NumHistoryShards)
	client.OverrideValue(dynamicconfig.ExecutionMgrNumConns, c.historyConfig.NumHistoryShards)
	client.OverrideValue(dynamicconfig.ReplicationTaskProcessorStartWait, time.Nanosecond)

	if c.workerConfig.EnableIndexer {
		client.OverrideValue(dynamicconfig.AdvancedVisibilityWritingMode, common.AdvancedVisibilityWritingModeDual)
	}
	if c.historyConfig.HistoryCountLimitWarn != 0 {
		client.OverrideValue(dynamicconfig.HistoryCountLimitWarn, c.historyConfig.HistoryCountLimitWarn)
	}
	if c.historyConfig.HistoryCountLimitError != 0 {
		client.OverrideValue(dynamicconfig.HistoryCountLimitError, c.historyConfig.HistoryCountLimitError)
	}
}

// copyPersistenceConfig makes a deepcopy of persistence config.
// This is just a temp fix for the race condition of persistence config.
// The race condition happens because all the services are using the same datastore map in the config.
// Also all services will retry to modify the maxQPS field in the datastore during start up and use the modified maxQPS value to create a persistence factory.
func copyPersistenceConfig(pConfig config.Persistence) (config.Persistence, error) {
	copiedDataStores := make(map[string]config.DataStore)
	for name, value := range pConfig.DataStores {
		copiedDataStore := config.DataStore{}
		encodedDataStore, err := json.Marshal(value)
		if err != nil {
			return pConfig, err
		}

		if err = json.Unmarshal(encodedDataStore, &copiedDataStore); err != nil {
			return pConfig, err
		}
		copiedDataStores[name] = copiedDataStore
	}
	pConfig.DataStores = copiedDataStores
	return pConfig, nil
}

func newMembershipResolver(serviceName string, hosts map[string][]membership.HostInfo) membership.Resolver {
	return NewSimpleResolver(serviceName, hosts)
}

func newPProfInitializerImpl(logger log.Logger, port int) common.PProfInitializer {
	return &config.PProfInitializerImpl{
		PProf: &config.PProf{
			Port: port,
		},
		Logger: logger,
	}
}

func newPublicClient(dispatcher *yarpc.Dispatcher) cwsc.Interface {
	config := dispatcher.ClientConfig(rpc.OutboundPublicClient)
	return compatibility.NewThrift2ProtoAdapter(
		apiv1.NewDomainAPIYARPCClient(config),
		apiv1.NewWorkflowAPIYARPCClient(config),
		apiv1.NewWorkerAPIYARPCClient(config),
		apiv1.NewVisibilityAPIYARPCClient(config),
	)
}

func (c *cadenceImpl) newRPCFactory(serviceName string, host membership.HostInfo) common.RPCFactory {
	tchannelAddress, err := host.GetNamedAddress(membership.PortTchannel)
	if err != nil {
		c.logger.Fatal("failed to get PortTchannel port from host", tag.Value(host), tag.Error(err))
	}

	grpcAddress, err := host.GetNamedAddress(membership.PortGRPC)
	if err != nil {
		c.logger.Fatal("failed to get PortGRPC port from host", tag.Value(host), tag.Error(err))
	}

	frontendGrpcAddress, err := c.FrontendHost().GetNamedAddress(membership.PortGRPC)
	if err != nil {
		c.logger.Fatal("failed to get frontend PortGRPC", tag.Value(c.FrontendHost()), tag.Error(err))
	}

	return rpc.NewFactory(c.logger, rpc.Params{
		ServiceName:     serviceName,
		TChannelAddress: tchannelAddress,
		GRPCAddress:     grpcAddress,
		InboundMiddleware: yarpc.InboundMiddleware{
			Unary: &versionMiddleware{},
		},

		// For integration tests to generate client out of the same outbound.
		OutboundsBuilder: rpc.CombineOutbounds(
			&singleGRPCOutbound{testOutboundName(serviceName), serviceName, grpcAddress},
			&singleGRPCOutbound{rpc.OutboundPublicClient, service.Frontend, frontendGrpcAddress},
			rpc.NewCrossDCOutbounds(c.clusterMetadata.GetAllClusterInfo(), rpc.NewDNSPeerChooserFactory(0, c.logger)),
			rpc.NewDirectOutbound(service.History, true, nil),
			rpc.NewDirectOutbound(service.Matching, true, nil),
		),
	})
}

// testOutbound prefixes outbound with "test-" to not clash with other real Cadence outbounds.
func testOutboundName(name string) string {
	return "test-" + name
}

type singleGRPCOutbound struct {
	outboundName string
	serviceName  string
	address      string
}

func (b singleGRPCOutbound) Build(grpc *grpc.Transport, _ *tchannel.Transport) (yarpc.Outbounds, error) {
	return yarpc.Outbounds{
		b.outboundName: {
			ServiceName: b.serviceName,
			Unary:       grpc.NewSingleOutbound(b.address),
		},
	}, nil
}

type versionMiddleware struct {
}

func (vm *versionMiddleware) Handle(ctx context.Context, req *transport.Request, resw transport.ResponseWriter, h transport.UnaryHandler) error {
	req.Headers = req.Headers.With(common.LibraryVersionHeaderName, "1.0.0").
		With(common.FeatureVersionHeaderName, cc.SupportedGoSDKVersion).
		With(common.ClientImplHeaderName, cc.GoSDK)

	return h.Handle(ctx, req, resw)
}
