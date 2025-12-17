package v1alpha2

import (
	unsafe "unsafe"

	porch "github.com/nephio-project/porch/api/porch"
	"k8s.io/apimachinery/pkg/conversion"
)

func Convert_porch_PackageRevisionSpec_To_v1alpha2_PackageRevisionSpec(in *porch.PackageRevisionSpec, out *PackageRevisionSpec, s conversion.Scope) error {
	out.PackageName = in.PackageName
	out.RepositoryName = in.RepositoryName
	out.WorkspaceName = in.WorkspaceName
	out.Revision = in.Revision
	out.Lifecycle = PackageRevisionLifecycle(in.Lifecycle)
	out.Tasks = *(*[]Task)(unsafe.Pointer(&in.Tasks))
	out.ReadinessGates = *(*[]ReadinessGate)(unsafe.Pointer(&in.ReadinessGates))
	return nil
}
