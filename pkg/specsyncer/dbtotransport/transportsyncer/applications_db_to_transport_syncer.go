package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/hub-of-hubs-all-in-one/pkg/bundle"
	"github.com/stolostron/hub-of-hubs-all-in-one/pkg/db"
	"github.com/stolostron/hub-of-hubs-all-in-one/pkg/intervalpolicy"
	"github.com/stolostron/hub-of-hubs-all-in-one/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1beta1 "sigs.k8s.io/application/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	applicationsTableName = "applications"
	applicationsMsgKey    = "Applications"
)

// AddApplicationsDBToTransportSyncer adds applications db to transport syncer to the manager.
func AddApplicationsDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, transportObj transport.Transport,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &appsv1beta1.Application{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("applications-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, transportObj, applicationsMsgKey, specDB, applicationsTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add applications db to transport syncer - %w", err)
	}

	return nil
}
