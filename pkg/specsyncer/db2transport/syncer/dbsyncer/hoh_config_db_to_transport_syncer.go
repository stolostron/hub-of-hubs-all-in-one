package dbsyncer

import (
	"context"
	"fmt"
	"time"

	configv1 "github.com/stolostron/hub-of-hubs-manager/pkg/apis/config/v1"
	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	configTableName = "configs"
	configMsgKey    = "Config"
)

// AddHoHConfigDBToTransportSyncer adds hub-of-hubs config db to transport syncer to the manager.
func AddHoHConfigDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, transportObj transport.Transport,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &configv1.Config{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("hoh-config-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, transportObj, configMsgKey, specDB, configTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add config db to transport syncer - %w", err)
	}

	return nil
}
