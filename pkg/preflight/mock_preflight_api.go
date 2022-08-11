// Code generated by MockGen. DO NOT EDIT.
// Source: preflight.go

// Package preflight is a generated GoMock package.
package preflight

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1beta1 "github.com/openshift/special-resource-operator/api/v1beta1"
	runtime "github.com/openshift/special-resource-operator/pkg/runtime"
)

// MockPreflightAPI is a mock of PreflightAPI interface.
type MockPreflightAPI struct {
	ctrl     *gomock.Controller
	recorder *MockPreflightAPIMockRecorder
}

// MockPreflightAPIMockRecorder is the mock recorder for MockPreflightAPI.
type MockPreflightAPIMockRecorder struct {
	mock *MockPreflightAPI
}

// NewMockPreflightAPI creates a new mock instance.
func NewMockPreflightAPI(ctrl *gomock.Controller) *MockPreflightAPI {
	mock := &MockPreflightAPI{ctrl: ctrl}
	mock.recorder = &MockPreflightAPIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPreflightAPI) EXPECT() *MockPreflightAPIMockRecorder {
	return m.recorder
}

// PreflightUpgradeCheck mocks base method.
func (m *MockPreflightAPI) PreflightUpgradeCheck(ctx context.Context, sr *v1beta1.SpecialResource, runInfo *runtime.RuntimeInformation) (bool, string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PreflightUpgradeCheck", ctx, sr, runInfo)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(string)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// PreflightUpgradeCheck indicates an expected call of PreflightUpgradeCheck.
func (mr *MockPreflightAPIMockRecorder) PreflightUpgradeCheck(ctx, sr, runInfo interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PreflightUpgradeCheck", reflect.TypeOf((*MockPreflightAPI)(nil).PreflightUpgradeCheck), ctx, sr, runInfo)
}

// PrepareRuntimeInfo mocks base method.
func (m *MockPreflightAPI) PrepareRuntimeInfo(ctx context.Context, image string) (*runtime.RuntimeInformation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PrepareRuntimeInfo", ctx, image)
	ret0, _ := ret[0].(*runtime.RuntimeInformation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PrepareRuntimeInfo indicates an expected call of PrepareRuntimeInfo.
func (mr *MockPreflightAPIMockRecorder) PrepareRuntimeInfo(ctx, image interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PrepareRuntimeInfo", reflect.TypeOf((*MockPreflightAPI)(nil).PrepareRuntimeInfo), ctx, image)
}