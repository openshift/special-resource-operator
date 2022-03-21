// Code generated by MockGen. DO NOT EDIT.
// Source: runtime.go

// Package runtime is a generated GoMock package.
package runtime

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
)

// MockRuntimeAPI is a mock of RuntimeAPI interface.
type MockRuntimeAPI struct {
	ctrl     *gomock.Controller
	recorder *MockRuntimeAPIMockRecorder
}

// MockRuntimeAPIMockRecorder is the mock recorder for MockRuntimeAPI.
type MockRuntimeAPIMockRecorder struct {
	mock *MockRuntimeAPI
}

// NewMockRuntimeAPI creates a new mock instance.
func NewMockRuntimeAPI(ctrl *gomock.Controller) *MockRuntimeAPI {
	mock := &MockRuntimeAPI{ctrl: ctrl}
	mock.recorder = &MockRuntimeAPIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRuntimeAPI) EXPECT() *MockRuntimeAPIMockRecorder {
	return m.recorder
}

// GetRuntimeInformation mocks base method.
func (m *MockRuntimeAPI) GetRuntimeInformation(ctx context.Context, sr *v1beta1.SpecialResource) (*RuntimeInformation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRuntimeInformation", ctx, sr)
	ret0, _ := ret[0].(*RuntimeInformation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRuntimeInformation indicates an expected call of GetRuntimeInformation.
func (mr *MockRuntimeAPIMockRecorder) GetRuntimeInformation(ctx, sr interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRuntimeInformation", reflect.TypeOf((*MockRuntimeAPI)(nil).GetRuntimeInformation), ctx, sr)
}

// LogRuntimeInformation mocks base method.
func (m *MockRuntimeAPI) LogRuntimeInformation(info *RuntimeInformation) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "LogRuntimeInformation", info)
}

// LogRuntimeInformation indicates an expected call of LogRuntimeInformation.
func (mr *MockRuntimeAPIMockRecorder) LogRuntimeInformation(info interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LogRuntimeInformation", reflect.TypeOf((*MockRuntimeAPI)(nil).LogRuntimeInformation), info)
}
