// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/spf13/pflag"
	"github.com/stolostron/hub-of-hubs-manager/pkg/compressor"
	"github.com/stolostron/hub-of-hubs-manager/pkg/nonk8sapi"
	"github.com/stolostron/hub-of-hubs-manager/pkg/scheme"
	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/db/postgresql"
	specsyncer "github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/syncer"
	spectransport "github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/transport"
	speckafka "github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/transport/kafka"
	specsyncservice "github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/transport/syncservice"
	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/spec2db"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statistics"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/db2status"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/status"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/db/workerpool"
	statussyncer "github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/syncer"
	statustransport "github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/transport"
	statuskafka "github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/transport/kafka"
	statussyncservice "github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/transport/syncservice"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	metricsHost                        = "0.0.0.0"
	metricsPort                  int32 = 8384
	kafkaTransportTypeName             = "kafka"
	syncServiceTransportTypeName       = "sync-service"
	leaderElectionLockName             = "hub-of-hubs-manager-lock"
)

var (
	errFlagParameterEmpty        = errors.New("flag parameter empty")
	errFlagParameterIllegalValue = errors.New("flag parameter illegal value")
)

type hohManagerConfig struct {
	managerNamespace      string
	watchNamespace        string
	syncerConfig          *syncerConfig
	databaseConfig        *databaseConfig
	transportCommonConfig *transportCommonConfig
	kafkaConfig           *kafkaConfig
	syncServiceConfig     *statussyncservice.SyncServiceConfig
	statisticsConfig      *statistics.StatisticsConfig
	nonK8sAPIServerConfig *nonk8sapi.NonK8sAPIServerConfig
}

type syncerConfig struct {
	specSyncInterval              time.Duration
	statusSyncInterval            time.Duration
	deletedLabelsTrimmingInterval time.Duration
}

type databaseConfig struct {
	processDatabaseURL         string
	transportBridgeDatabaseURL string
}

type transportCommonConfig struct {
	transportType      string
	msgCompressionType string
	committerInterval  time.Duration
}

type kafkaConfig struct {
	bootstrapServer string
	producerConfig  *speckafka.KafkaProducerConfig
	consumerConfig  *statuskafka.KafkaConsumerConfig
}

func parseFlags() (*hohManagerConfig, error) {
	managerConfig := &hohManagerConfig{
		syncerConfig:          &syncerConfig{},
		databaseConfig:        &databaseConfig{},
		transportCommonConfig: &transportCommonConfig{},
		kafkaConfig: &kafkaConfig{
			producerConfig: &speckafka.KafkaProducerConfig{},
			consumerConfig: &statuskafka.KafkaConsumerConfig{},
		},
		syncServiceConfig:     &statussyncservice.SyncServiceConfig{},
		statisticsConfig:      &statistics.StatisticsConfig{},
		nonK8sAPIServerConfig: &nonk8sapi.NonK8sAPIServerConfig{},
	}

	pflag.StringVar(&managerConfig.managerNamespace, "manager-namespace", "open-cluster-management", "The manager running namespace, also used as leader election namespace.")
	pflag.StringVar(&managerConfig.watchNamespace, "watch-namespace", "", "The watching namespace of the controllers, multiple namespace must be splited by comma.")
	pflag.DurationVar(&managerConfig.syncerConfig.specSyncInterval, "spec-sync-interval", 5*time.Second, "The synchronization interval of resources in spec.")
	pflag.DurationVar(&managerConfig.syncerConfig.statusSyncInterval, "status-sync-interval", 5*time.Second, "The synchronization interval of resources in status.")
	pflag.DurationVar(&managerConfig.syncerConfig.deletedLabelsTrimmingInterval, "deleted-labels-trimming-interval", 5*time.Second, "The trimming interval of deleted labels.")
	pflag.StringVar(&managerConfig.databaseConfig.processDatabaseURL, "process-database-url", "", "The URL of database server for the process user.")
	pflag.StringVar(&managerConfig.databaseConfig.transportBridgeDatabaseURL, "transport-bridge-database-url", "", "The URL of database server for the transport-bridge user.")
	pflag.StringVar(&managerConfig.transportCommonConfig.transportType, "transport-type", "kafka", "The transport type, 'kafka' or 'sync-service'.")
	pflag.StringVar(&managerConfig.transportCommonConfig.msgCompressionType, "transport-message-compression-type", "gzip", "The message compression type for transport layer, 'gzip' or 'no-op'.")
	pflag.DurationVar(&managerConfig.transportCommonConfig.committerInterval, "transport-committer-interval", 40*time.Second, "The committer interval for transport layer.")
	pflag.StringVar(&managerConfig.kafkaConfig.bootstrapServer, "kafka-bookstrap-server", "kafka-brokers-cluster-kafka-bootstrap.kafka.svc:9092", "The bootstrap server for kafka.")
	pflag.StringVar(&managerConfig.kafkaConfig.producerConfig.ProducerID, "kakfa-producer-id", "hub-of-hubs", "ID for the kafka producer.")
	pflag.StringVar(&managerConfig.kafkaConfig.producerConfig.ProducerTopic, "kakfa-producer-topic", "spec", "Topic for the kafka producer.")
	pflag.IntVar(&managerConfig.kafkaConfig.producerConfig.MsgSizeLimitKB, "kafka-message-size-limit", 940, "The limit for kafka message size in KB.")
	pflag.StringVar(&managerConfig.kafkaConfig.consumerConfig.ConsumerID, "kakfa-consumer-id", "hub-of-hubs", "ID for the kafka consumer.")
	pflag.StringVar(&managerConfig.kafkaConfig.consumerConfig.ConsumerTopic, "kakfa-consumer-topic", "status", "Topic for the kafka consumer.")
	pflag.StringVar(&managerConfig.syncServiceConfig.Protocol, "sync-service-protocol", "http", "The protocol for sync-service communication.")
	pflag.StringVar(&managerConfig.syncServiceConfig.CSSHost, "cloud-sync-service-host", "sync-service-css.sync-service.svc.cluster.local", "The host for Cloud Sync Service.")
	pflag.IntVar(&managerConfig.syncServiceConfig.CSSPort, "cloud-sync-service-port", 9689, "The port for Cloud Sync Service.")
	pflag.IntVar(&managerConfig.syncServiceConfig.PollingInterval, "cloud-sync-service-polling-interval", 5, "The polling interval in second for Cloud Sync Service.")
	pflag.DurationVar(&managerConfig.statisticsConfig.LogInterval, "statistics-log-interval", 0*time.Second, "The log interval for statistics.")
	pflag.StringVar(&managerConfig.nonK8sAPIServerConfig.ClusterAPIURL, "cluster-api-url", "https://kubernetes.default.svc:443", "The cluster API URL for nonK8s API server.")
	pflag.StringVar(&managerConfig.nonK8sAPIServerConfig.ClusterAPICABundlePath, "cluster-api-cabundle-path", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "The CA bundle path for cluster API.")
	pflag.StringVar(&managerConfig.nonK8sAPIServerConfig.AuthorizationURL, "authorization-url", "https://hub-of-hubs-rbac.open-cluster-management.svc:8181", "The authorization URL for nonK8s API server.")
	pflag.StringVar(&managerConfig.nonK8sAPIServerConfig.AuthorizationCABundlePath, "authorization-cabundle-path", "/hub-of-hubs-rbac-ca/service-ca.crt", "The CA bundle path for authorization server.")
	pflag.StringVar(&managerConfig.nonK8sAPIServerConfig.ServerCertificatePath, "server-certificate-path", "/certs/tls.crt", "The certificate path for nonK8s API server.")
	pflag.StringVar(&managerConfig.nonK8sAPIServerConfig.ServerKeyPath, "server-key-path", "/certs/tls.key", "The private key path for nonK8s API server.")
	pflag.StringVar(&managerConfig.nonK8sAPIServerConfig.ServerBasePath, "server-base-path", "/multicloud/hub-of-hubs-nonk8s-api", "The base path for nonK8s API server.")

	// add flags for logger
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	if managerConfig.databaseConfig.processDatabaseURL == "" {
		return nil, fmt.Errorf("database url for process user: %w", errFlagParameterEmpty)
	}

	if managerConfig.databaseConfig.transportBridgeDatabaseURL == "" {
		return nil, fmt.Errorf("database url for transport-bridge user: %w", errFlagParameterEmpty)
	}

	if managerConfig.kafkaConfig.producerConfig.MsgSizeLimitKB > speckafka.MaxMessageSizeLimit {
		return nil, fmt.Errorf("%w - size must not exceed %d : %s", errFlagParameterIllegalValue,
			speckafka.MaxMessageSizeLimit, "kafka-message-size-limit")
	}

	return managerConfig, nil
}

func initializeLogger() logr.Logger {
	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")

	return log
}

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
}

// function to determine whether the transport component requires initial-dependencies between bundles to be checked
// (on load). If the returned is false, then we may assume that dependency of the initial bundle of
// each type is met. Otherwise, there are no guarantees and the dependencies must be checked.
func requireInitialDependencyChecks(transportType string) bool {
	switch transportType {
	case kafkaTransportTypeName:
		return false
		// once kafka consumer loads up, it starts reading from the earliest un-processed bundle,
		// as in all bundles that precede the latter have been processed, which include its dependency
		// bundle.

		// the order guarantee also guarantees that if while loading this component, a new bundle is written to a-
		// partition, then surely its dependency was written before it (leaf-hub-status-sync on kafka guarantees).
	case syncServiceTransportTypeName:
		fallthrough
	default:
		return true
	}
}

// function to choose spec transport type based on env var.
func getSpecTransport(transportCommonConfig *transportCommonConfig, kafkaBootstrapServer string, kafkaProducerConfig *speckafka.KafkaProducerConfig, syncServiceConfig *statussyncservice.SyncServiceConfig) (spectransport.Transport, error) {
	msgCompressor, err := compressor.NewCompressor(compressor.CompressionType(transportCommonConfig.msgCompressionType))
	if err != nil {
		return nil, fmt.Errorf("failed to create message-compressor: %w", err)
	}

	switch transportCommonConfig.transportType {
	case kafkaTransportTypeName:
		kafkaProducer, err := speckafka.NewProducer(msgCompressor, kafkaBootstrapServer, kafkaProducerConfig, ctrl.Log.WithName("kafka-producer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-producer: %w", err)
		}

		return kafkaProducer, nil
	case syncServiceTransportTypeName:
		syncService, err := specsyncservice.NewSyncService(msgCompressor, syncServiceConfig, ctrl.Log.WithName("sync-service-for-spec"))
		if err != nil {
			return nil, fmt.Errorf("failed to create sync-service for spec: %w", err)
		}

		return syncService, nil
	default:
		return nil, fmt.Errorf("%w: %s - %s is not a valid option", errFlagParameterIllegalValue, "transport-type",
			transportCommonConfig.transportType)
	}
}

// function to choose status transport type based on env var.
func getStatusTransport(transportCommonConfig *transportCommonConfig, kafkaBootstrapServer string, kafkaConsumerConfig *statuskafka.KafkaConsumerConfig, syncServiceConfig *statussyncservice.SyncServiceConfig, conflationMgr *conflator.ConflationManager,
	statistics *statistics.Statistics,
) (statustransport.Transport, error) {
	switch transportCommonConfig.transportType {
	case kafkaTransportTypeName:
		kafkaConsumer, err := statuskafka.NewConsumer(transportCommonConfig.committerInterval, kafkaBootstrapServer, kafkaConsumerConfig, conflationMgr, statistics, ctrl.Log.WithName("kafka-consumer"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-consumer: %w", err)
		}

		return kafkaConsumer, nil
	case syncServiceTransportTypeName:
		syncService, err := statussyncservice.NewSyncService(transportCommonConfig.committerInterval, syncServiceConfig, conflationMgr, statistics, ctrl.Log.WithName("sync-service-for-status"))
		if err != nil {
			return nil, fmt.Errorf("failed to create sync-service for status: %w", err)
		}

		return syncService, nil
	default:
		return nil, fmt.Errorf("%w: %s - %s is not a valid option", errFlagParameterIllegalValue, "transport-type",
			transportCommonConfig.transportType)
	}
}

func createManager(managerConfig *hohManagerConfig, processPostgreSQL, transportBridgePostgreSQL *postgresql.PostgreSQL, workersPool *workerpool.DBWorkerPool,
	specTransportObj spectransport.Transport, statusTransportObj statustransport.Transport, conflationManager *conflator.ConflationManager,
	conflationReadyQueue *conflator.ConflationReadyQueue, statistics *statistics.Statistics,
) (ctrl.Manager, error) {
	options := ctrl.Options{
		Namespace:               managerConfig.watchNamespace,
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		LeaderElection:          true,
		LeaderElectionNamespace: managerConfig.managerNamespace,
		LeaderElectionID:        leaderElectionLockName,
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if strings.Contains(managerConfig.watchNamespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(managerConfig.watchNamespace, ","))
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	if err := scheme.AddToScheme(mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to add schemes: %w", err)
	}

	if err := nonk8sapi.AddNonK8sApiServer(mgr, processPostgreSQL, managerConfig.nonK8sAPIServerConfig); err != nil {
		return nil, fmt.Errorf("failed to add non-k8s-api-server: %w", err)
	}

	if err := spec2db.AddSpec2DBControllers(mgr, processPostgreSQL); err != nil {
		return nil, fmt.Errorf("failed to add spec-to-db controllers: %w", err)
	}

	if err := specsyncer.AddDB2TransportSyncers(mgr, transportBridgePostgreSQL, specTransportObj, managerConfig.syncerConfig.specSyncInterval); err != nil {
		return nil, fmt.Errorf("failed to add db-to-transport syncers: %w", err)
	}

	if err := specsyncer.AddStatusDBWatchers(mgr, transportBridgePostgreSQL, transportBridgePostgreSQL, managerConfig.syncerConfig.deletedLabelsTrimmingInterval); err != nil {
		return nil, fmt.Errorf("failed to add status db watchers: %w", err)
	}

	if err := db2status.AddDBSyncers(mgr, processPostgreSQL, managerConfig.syncerConfig.statusSyncInterval); err != nil {
		return nil, fmt.Errorf("failed to add status db syncers: %w", err)
	}

	if err := status.AddStatusControllers(mgr); err != nil {
		return nil, fmt.Errorf("failed to add status controller: %w", err)
	}

	if err := statussyncer.AddTransport2DBSyncers(mgr, workersPool, conflationManager, conflationReadyQueue, statusTransportObj, statistics); err != nil {
		return nil, fmt.Errorf("failed to add transport-to-db syncers: %w", err)
	}

	return mgr, nil
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain() int {
	log := initializeLogger()
	printVersion(log)
	// create hoh manager configuration from command parameters
	managerConfig, err := parseFlags()
	if err != nil {
		log.Error(err, "flags parse error")
		return 1
	}

	// create statistics
	stats, err := statistics.NewStatistics(ctrl.Log.WithName("statistics"), managerConfig.statisticsConfig)
	if err != nil {
		log.Error(err, "initialization error", "failed to initialize", "statistics")
		return 1
	}

	// db layer initialization for process user
	processPostgreSQL, err := postgresql.NewPostgreSQL(managerConfig.databaseConfig.processDatabaseURL)
	if err != nil {
		log.Error(err, "initialization error", "failed to initialize", "process PostgreSQL")
		return 1
	}
	defer processPostgreSQL.Stop()

	// db layer initialization for transport-bridge user
	transportBridgePostgreSQL, err := postgresql.NewPostgreSQL(managerConfig.databaseConfig.transportBridgeDatabaseURL)
	if err != nil {
		log.Error(err, "initialization error", "failed to initialize", "transport-bridge PostgreSQL")
		return 1
	}
	defer transportBridgePostgreSQL.Stop()

	// db layer initialization - worker pool + connection pool
	dbWorkerPool, err := workerpool.NewDBWorkerPool(ctrl.Log.WithName("db-worker-pool"), managerConfig.databaseConfig.transportBridgeDatabaseURL, stats)
	if err != nil {
		log.Error(err, "initialization error", "failed to initialize", "DBWorkerPool")
		return 1
	}

	if err = dbWorkerPool.Start(); err != nil {
		log.Error(err, "initialization error", "failed to start", "DBWorkerPool")
		return 1
	}
	defer dbWorkerPool.Stop()

	// conflationReadyQueue is shared between conflation manager and dispatcher
	conflationReadyQueue := conflator.NewConflationReadyQueue(stats)
	requireInitialDependencyChecks := requireInitialDependencyChecks(managerConfig.transportCommonConfig.transportType)
	conflationManager := conflator.NewConflationManager(ctrl.Log.WithName("conflation"), conflationReadyQueue,
		requireInitialDependencyChecks, stats) // manage all Conflation Units

	// status transport layer initialization
	statusTransportObj, err := getStatusTransport(managerConfig.transportCommonConfig, managerConfig.kafkaConfig.bootstrapServer, managerConfig.kafkaConfig.consumerConfig, managerConfig.syncServiceConfig, conflationManager, stats)
	if err != nil {
		log.Error(err, "initialization error", "failed to initialize", "status transport")
		return 1
	}

	statusTransportObj.Start()
	defer statusTransportObj.Stop()

	// spec transport layer initialization
	specTransportObj, err := getSpecTransport(managerConfig.transportCommonConfig, managerConfig.kafkaConfig.bootstrapServer, managerConfig.kafkaConfig.producerConfig, managerConfig.syncServiceConfig)
	if err != nil {
		log.Error(err, "initialization error", "failed to initialize", "spec transport")
		return 1
	}

	specTransportObj.Start()
	defer specTransportObj.Stop()

	mgr, err := createManager(managerConfig, processPostgreSQL, transportBridgePostgreSQL, dbWorkerPool, specTransportObj, statusTransportObj, conflationManager, conflationReadyQueue, stats)
	if err != nil {
		log.Error(err, "failed to create manager")
		return 1
	}

	log.Info("Starting the Cmd.")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited non-zero")
		return 1
	}

	return 0
}

func main() {
	os.Exit(doMain())
}
