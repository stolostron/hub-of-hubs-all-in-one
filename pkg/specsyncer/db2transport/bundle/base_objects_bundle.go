package bundle

import (
	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewBaseObjectsBundle creates a new base bundle with no data in it.
func NewBaseObjectsBundle() ObjectsBundle {
	return &baseObjectsBundle{
		Objects:        make([]metav1.Object, 0),
		DeletedObjects: make([]metav1.Object, 0),
	}
}

type baseObjectsBundle struct {
	Objects        []metav1.Object `json:"objects"`
	DeletedObjects []metav1.Object `json:"deletedObjects"`
}

// AddObject adds an object to the bundle.
func (b *baseObjectsBundle) AddObject(object metav1.Object, objectUID string) {
	setMetaDataAnnotation(object, datatypes.OriginOwnerReferenceAnnotation, objectUID)
	b.Objects = append(b.Objects, object)
}

// AddDeletedObject adds a deleted object to the bundle.
func (b *baseObjectsBundle) AddDeletedObject(object metav1.Object) {
	b.DeletedObjects = append(b.DeletedObjects, object)
}

// setMetaDataAnnotation sets metadata annotation on the given object.
func setMetaDataAnnotation(object metav1.Object, key string, value string) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[key] = value

	object.SetAnnotations(annotations)
}
