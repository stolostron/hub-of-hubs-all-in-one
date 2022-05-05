// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package spec2db

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs-all-in-one/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/hub-of-hubs-all-in-one/pkg/specsyncer/spec2db/controller"
	ctrl "sigs.k8s.io/controller-runtime"
)

// AddSpec2DBControllers adds all the spec-to-db controllers to the Manager.
func AddSpec2DBControllers(mgr ctrl.Manager, specDB db.SpecDB) error {
	addControllerFunctions := []func(ctrl.Manager, db.SpecDB) error{
		controller.AddPolicyController,
		controller.AddPlacementRuleController,
		controller.AddPlacementBindingController,
		controller.AddHubOfHubsConfigController,
		controller.AddApplicationController,
		controller.AddSubscriptionController,
		controller.AddChannelController,
		controller.AddManagedClusterSetController,
		controller.AddManagedClusterSetBindingController,
		controller.AddPlacementController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr, specDB); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
