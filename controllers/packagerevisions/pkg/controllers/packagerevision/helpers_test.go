package packagerevision

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// fakeManager is a minimal ctrl.Manager for unit testing Init().
// Only GetClient() is implemented; all other methods will panic if called.
type fakeManager struct {
	manager.Manager
	client client.Client
}

func (f *fakeManager) GetClient() client.Client {
	return f.client
}
