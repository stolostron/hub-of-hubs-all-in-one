package dbsyncer

import (
	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs-manager/pkg/bundle/status"
	"github.com/stolostron/hub-of-hubs-manager/pkg/constants"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/db"
)

// NewSubscriptionReportsDBSyncer creates a new instance of genericDBSyncer to sync subscription-reports.
func NewSubscriptionReportsDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &genericDBSyncer{
		log:              log,
		transportMsgKey:  constants.SubscriptionReportMsgKey,
		dbSchema:         db.StatusSchema,
		dbTableName:      db.SubscriptionReportsTableName,
		createBundleFunc: bundle.NewSubscriptionReportsBundle,
		bundlePriority:   conflator.SubscriptionReportPriority,
		bundleSyncMode:   status.CompleteStateMode,
	}

	log.Info("initialized subscription-reports db syncer")

	return dbSyncer
}
