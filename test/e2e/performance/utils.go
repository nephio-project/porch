package performance

import (
	"fmt"
	"reflect"

	porchapiv1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
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
	// is regular type
	if name := reflect.TypeOf(o).Name(); name != "" {
		return name
	}
	// is pointer
	return reflect.TypeOf(o).Elem().Name()
}

func NameOf(o any) string {
	switch c := o.(type) {
	case *porchapiv1alpha1.PackageRevision:
		return fmt.Sprintf("%s/%s", c.Spec.RepositoryName, c.Spec.PackageName)
	case *porchapiv1alpha1.PackageRevisionResources:
		return fmt.Sprintf("%s/%s", c.Spec.RepositoryName, c.Spec.PackageName)
	case *configapi.Repository:
		return c.Name
	case hasName:
		return c.GetName()
	default:
		return ""
	}
}
