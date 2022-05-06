package conflator

import (
	"github.com/stolostron/hub-of-hubs-manager/pkg/bundle/status"
	"github.com/stolostron/hub-of-hubs-manager/pkg/statussyncer/transport2db/transport"
)

// BundleMetadata abstracts metadata of conflation elements inside the conflation units.
type BundleMetadata struct {
	bundleType    string
	bundleVersion *status.BundleVersion
	// transport metadata is information we need for marking bundle as consumed in transport (e.g. commit offset)
	transportBundleMetadata transport.BundleMetadata
}
