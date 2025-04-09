package performance

import (
	"reflect"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type hasName interface {
	GetName() string
}

func KindOf(o any) string {
	if c, ok := o.(client.Object); ok {
		if kind := c.GetObjectKind().GroupVersionKind().Kind; kind != "" {
			return kind
		}
	}
	return reflect.TypeOf(o).Elem().Name()
}

func NameOf(o any) string {
	switch c := o.(type) {
	case *porchapi.PackageRevision:
		return c.Spec.PackageName
	case *porchapi.PackageRevisionResources:
		return c.Spec.PackageName
	case *configapi.Repository:
		return c.Name
	case hasName:
		return c.GetName()
	default:
		return ""
	}
}
