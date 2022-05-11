// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs-manager/pkg/specsyncer/db2transport/db"
	"k8s.io/apimachinery/pkg/api/equality"
	subscriptionv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func AddSubscriptionController(mgr ctrl.Manager, specDB db.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&subscriptionv1.Subscription{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return object.GetNamespace() != "open-cluster-management"
		})).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("subscriptions-spec-syncer"),
			tableName:      "subscriptions",
			finalizerName:  hohCleanupFinalizer,
			createInstance: func() client.Object { return &subscriptionv1.Subscription{} },
			cleanStatus:    cleanSubscriptionStatus,
			areEqual:       areSubscriptionsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add subscription controller to the manager: %w", err)
	}

	return nil
}

func cleanSubscriptionStatus(instance client.Object) {
	subscription, ok := instance.(*subscriptionv1.Subscription)
	if !ok {
		panic("wrong instance passed to cleanSubscriptionStatus: not a Subscription")
	}

	subscription.Status = subscriptionv1.SubscriptionStatus{}
}

func areSubscriptionsEqual(instance1, instance2 client.Object) bool {
	// TODO: subscription come out as not equal because of package override field, check if it matters.
	subscription1, ok1 := instance1.(*subscriptionv1.Subscription)
	subscription2, ok2 := instance2.(*subscriptionv1.Subscription)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(subscription1.Spec, subscription2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
