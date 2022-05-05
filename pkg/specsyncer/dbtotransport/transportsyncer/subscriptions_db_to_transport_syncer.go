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
	subscriptionsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	subscriptionsTableName = "subscriptions"
	subscriptionMsgKey     = "Subscriptions"
)

// AddSubscriptionsDBToTransportSyncer adds subscriptions db to transport syncer to the manager.
func AddSubscriptionsDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, transportObj transport.Transport,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &subscriptionsv1.Subscription{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("subscriptions-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, transportObj, subscriptionMsgKey, specDB, subscriptionsTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add subscriptions db to transport syncer - %w", err)
	}

	return nil
}
