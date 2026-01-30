/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package meta

import (
	"context"
	"errors"
	"testing"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/internal/api/porchinternal/v1alpha1"
	internalapi "github.com/nephio-project/porch/internal/api/porchinternal/v1alpha1"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCreateUpdateDeleteOK(t *testing.T) {
	mockClient := mockclient.NewMockClient(t)
	ctxt := context.TODO()

	store := NewCrdMetadataStore(mockClient)
	assert.Equal(t, mockClient, store.coreClient)

	repo := configapi.Repository{}
	pkgRevMeta := metav1.ObjectMeta{
		Name:      "my-name",
		Namespace: "my-namespace",
	}

	mockClient.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
	newPkgRevMeta, err := store.Create(ctxt, pkgRevMeta, repo.Name, uuid.NewUUID())
	assert.True(t, err == nil)
	assert.Equal(t, pkgRevMeta.Name, newPkgRevMeta.Name)

	prKey := types.NamespacedName{
		Name:      pkgRevMeta.Name,
		Namespace: pkgRevMeta.Namespace,
	}
	internalPkgRev := v1alpha1.PackageRev{}

	mockClient.EXPECT().
		Get(mock.Anything, prKey, &internalPkgRev).
		Return(nil).
		Run(func(_ context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) {
			obj.(*v1alpha1.PackageRev).Name = prKey.Name
			obj.(*v1alpha1.PackageRev).Namespace = prKey.Namespace
		})
	gotPR, err := store.Get(ctxt, prKey)
	assert.True(t, err == nil)
	assert.Equal(t, gotPR.Name, prKey.Name)

	internalPkgRevList := internalapi.PackageRevList{}
	mockClient.EXPECT().
		List(mock.Anything, &internalPkgRevList, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(_ context.Context, list client.ObjectList, opts ...client.ListOption) {
			list.(*v1alpha1.PackageRevList).Items = make([]v1alpha1.PackageRev, 1)
			list.(*v1alpha1.PackageRevList).Items[0] = v1alpha1.PackageRev{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: pkgRevMeta.Namespace,
					Name:      pkgRevMeta.Name,
				},
			}
		})
	prList, err := store.List(ctxt, &repo)
	assert.True(t, err == nil)
	assert.Equal(t, prList[0].Name, prKey.Name)

	mockClient.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
	updPkgRevMeta, err := store.Update(ctxt, newPkgRevMeta)
	assert.True(t, err == nil)
	assert.Equal(t, pkgRevMeta.Name, updPkgRevMeta.Name)

	mockClient.EXPECT().Delete(mock.Anything, mock.Anything).Return(nil)
	delPkgRevMeta, err := store.Delete(ctxt, prKey, true)
	assert.True(t, err == nil)
	assert.Equal(t, delPkgRevMeta.Name, prKey.Name)
}

func TestCreateAlreadyExists(t *testing.T) {
	mockClient := mockclient.NewMockClient(t)
	ctxt := context.TODO()

	store := NewCrdMetadataStore(mockClient)
	assert.Equal(t, mockClient, store.coreClient)

	repo := configapi.Repository{}
	pkgRevMeta := metav1.ObjectMeta{
		Name:      "my-name",
		Namespace: "my-namespace",
	}

	expectedError := apierrors.NewAlreadyExists(schema.GroupResource{}, "any-name")
	mockClient.EXPECT().Create(mock.Anything, mock.Anything).Return(expectedError)
	mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("get error1"))
	_, err := store.Create(ctxt, pkgRevMeta, repo.Name, uuid.NewUUID())
	assert.True(t, err != nil)
}

func TestStoreErrors(t *testing.T) {
	mockClient := mockclient.NewMockClient(t)
	ctxt := context.TODO()

	store := NewCrdMetadataStore(mockClient)
	assert.Equal(t, mockClient, store.coreClient)

	nsn := types.NamespacedName{}
	mockClient.EXPECT().Get(mock.Anything, nsn, mock.Anything).Return(errors.New("get error1"))
	_, err := store.Get(ctxt, nsn)
	assert.True(t, err != nil)

	repo := configapi.Repository{}
	mockClient.EXPECT().List(mock.Anything, mock.Anything, client.InNamespace(""), mock.Anything).Return(errors.New("get error"))
	_, err = store.List(ctxt, &repo)
	assert.True(t, err != nil)

	pkgRevMeta := metav1.ObjectMeta{}
	mockClient.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("get error2"))
	_, err = store.Create(ctxt, pkgRevMeta, repo.Name, uuid.NewUUID())
	assert.True(t, err != nil)

	mockClient.EXPECT().Get(mock.Anything, nsn, mock.Anything).Return(errors.New("get error3"))
	_, err = store.Delete(ctxt, nsn, true)
	assert.True(t, err != nil)
}

func TestUpdateStatus(t *testing.T) {
	tests := []struct {
		name         string
		renderStatus *porchapi.RenderStatus
		getErr       error
		updateErr    error
		wantErr      bool
		validate     func(*testing.T, *v1alpha1.PackageRev)
	}{
		{
			name: "success with render error",
			renderStatus: &porchapi.RenderStatus{
				Err: "render error",
				Result: porchapi.ResultList{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "should-be-cleared",
						Namespace: "should-be-cleared",
					},
				},
			},
			validate: func(t *testing.T, pkgRev *v1alpha1.PackageRev) {
				assert.NotNil(t, pkgRev.Status.RenderStatus)
				assert.Equal(t, "render error", pkgRev.Status.RenderStatus.Err)
				assert.Empty(t, pkgRev.Status.RenderStatus.Result.ObjectMeta.Name)
				assert.Empty(t, pkgRev.Status.RenderStatus.Result.ObjectMeta.Namespace)
			},
		},
		{
			name:         "success with nil render status",
			renderStatus: nil,
			validate: func(t *testing.T, pkgRev *v1alpha1.PackageRev) {
				assert.Nil(t, pkgRev.Status.RenderStatus)
			},
		},
		{
			name:         "get error",
			renderStatus: &porchapi.RenderStatus{},
			getErr:       errors.New("get error"),
			wantErr:      true,
		},
		{
			name:         "update error",
			renderStatus: &porchapi.RenderStatus{Err: "test error"},
			updateErr:    errors.New("update error"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			mockStatusWriter := mockclient.NewMockSubResourceWriter(t)
			ctx := context.TODO()

			store := NewCrdMetadataStore(mockClient)
			nsn := types.NamespacedName{Name: "test-pkg", Namespace: "test-ns"}

			mockClient.EXPECT().
				Get(ctx, nsn, mock.AnythingOfType("*v1alpha1.PackageRev")).
				Return(tt.getErr).
				Run(func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) {
					if tt.getErr == nil {
						obj.(*v1alpha1.PackageRev).Name = nsn.Name
						obj.(*v1alpha1.PackageRev).Namespace = nsn.Namespace
					}
				})

			if tt.getErr == nil {
				mockClient.EXPECT().Status().Return(mockStatusWriter)
				mockStatusWriter.EXPECT().
					Update(ctx, mock.AnythingOfType("*v1alpha1.PackageRev")).
					Return(tt.updateErr).
					Run(func(_ context.Context, obj client.Object, _ ...client.SubResourceUpdateOption) {
						if tt.validate != nil {
							tt.validate(t, obj.(*v1alpha1.PackageRev))
						}
					})
			}

			err := store.UpdateStatus(ctx, nsn, tt.renderStatus)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
