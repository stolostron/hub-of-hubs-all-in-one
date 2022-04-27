// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"github.com/jackc/pgx/v4/pgxpool"
	"k8s.io/apimachinery/pkg/api/equality"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func addManagedClusterSetBindingController(mgr ctrl.Manager, databaseConnectionPool *pgxpool.Pool) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1beta1.ManagedClusterSetBinding{}).
		Complete(&genericSpecToDBReconciler{
			client:                 mgr.GetClient(),
			databaseConnectionPool: databaseConnectionPool,
			log:                    ctrl.Log.WithName("managedclustersetbindings-spec-syncer"),
			tableName:              "managedclustersetbindings",
			finalizerName:          "hub-of-hubs.open-cluster-management.io/managedclustersetbinding-cleanup",
			createInstance:         func() client.Object { return &clusterv1beta1.ManagedClusterSetBinding{} },
			cleanStatus:            cleanManagedClusterSetBindingsStatus,
			areEqual:               areManagedClusterSetBindingsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add managed cluster set binding controller to the manager: %w", err)
	}

	return nil
}

func cleanManagedClusterSetBindingsStatus(instance client.Object) {
	_, ok := instance.(*clusterv1beta1.ManagedClusterSetBinding)
	// ManagedClusterSetBinding has no status
	if !ok {
		panic("wrong instance passed to cleanManagedClusterSetBindingsStatus: not a ManagedClusterSetBinding")
	}
}

func areManagedClusterSetBindingsEqual(instance1, instance2 client.Object) bool {
	managedClusterSetBinding1, ok1 := instance1.(*clusterv1beta1.ManagedClusterSetBinding)
	managedClusterSetBinding2, ok2 := instance2.(*clusterv1beta1.ManagedClusterSetBinding)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(managedClusterSetBinding1.Spec, managedClusterSetBinding2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
