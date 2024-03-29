// Code generated by MockGen. DO NOT EDIT.
// Source: openshift_ca.go

// Package registry is a generated GoMock package.
package registry

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockOpenShiftCAGetter is a mock of OpenShiftCAGetter interface.
type MockOpenShiftCAGetter struct {
	ctrl     *gomock.Controller
	recorder *MockOpenShiftCAGetterMockRecorder
}

// MockOpenShiftCAGetterMockRecorder is the mock recorder for MockOpenShiftCAGetter.
type MockOpenShiftCAGetterMockRecorder struct {
	mock *MockOpenShiftCAGetter
}

// NewMockOpenShiftCAGetter creates a new mock instance.
func NewMockOpenShiftCAGetter(ctrl *gomock.Controller) *MockOpenShiftCAGetter {
	mock := &MockOpenShiftCAGetter{ctrl: ctrl}
	mock.recorder = &MockOpenShiftCAGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockOpenShiftCAGetter) EXPECT() *MockOpenShiftCAGetterMockRecorder {
	return m.recorder
}

// AdditionalTrustedCAs mocks base method.
func (m *MockOpenShiftCAGetter) AdditionalTrustedCAs(ctx context.Context) (map[string][]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AdditionalTrustedCAs", ctx)
	ret0, _ := ret[0].(map[string][]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AdditionalTrustedCAs indicates an expected call of AdditionalTrustedCAs.
func (mr *MockOpenShiftCAGetterMockRecorder) AdditionalTrustedCAs(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AdditionalTrustedCAs", reflect.TypeOf((*MockOpenShiftCAGetter)(nil).AdditionalTrustedCAs), ctx)
}

// CABundle mocks base method.
func (m *MockOpenShiftCAGetter) CABundle(arg0 context.Context) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CABundle", arg0)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CABundle indicates an expected call of CABundle.
func (mr *MockOpenShiftCAGetterMockRecorder) CABundle(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CABundle", reflect.TypeOf((*MockOpenShiftCAGetter)(nil).CABundle), arg0)
}
