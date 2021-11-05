package kernel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const kernelFullVersion = "4.18.0-305.19.1.el8_4.x86_64"

func newObj(kind, name string) *unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetKind(kind)

	return &obj
}

func TestSetAffineAttributes(t *testing.T) {
	const (
		objName                   = "test-obj"
		objNameHash               = "bfb16b50984f16f0" // fnv64a(operatingSystemMajorMinor + kernelFullVersion)
		objNewName                = objName + "-" + objNameHash
		operatingSystemMajorMinor = "8.4"
	)

	t.Run("BuildRun", func(t *testing.T) {
		obj := newObj("BuildRun", objName)

		err := SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor)

		require.NoError(t, err)
		assert.Equal(t, objNewName, obj.GetName())
	})

	for _, kind := range []string{"Pod", "BuildConfig"} {
		t.Run(kind, func(t *testing.T) {
			obj := newObj(kind, objNewName)

			err := SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor)
			require.NoError(t, err)

			expectedSelector := map[string]interface{}{
				"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
			}

			v, ok, err := unstructured.NestedMap(obj.Object, "spec", "nodeSelector")
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, expectedSelector, v)
		})
	}

	for _, kind := range []string{"DaemonSet", "Deployment", "StatefulSet"} {
		t.Run(kind, func(t *testing.T) {
			obj := newObj(kind, objName)

			err := SetAffineAttributes(obj, kernelFullVersion, operatingSystemMajorMinor)
			require.NoError(t, err)

			assert.Equal(t, objNewName, obj.GetLabels()["app"])

			v, ok, err := unstructured.NestedString(obj.Object, "spec", "selector", "matchLabels", "app")
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, objNewName, v)

			v, ok, err = unstructured.NestedString(obj.Object, "spec", "template", "metadata", "labels", "app")
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, objNewName, v)

			// one if compares the kind to StatefulSet, the other one to StatefulSet (capital S)
			if kind != "StatefulSet" {
				expectedSelector := map[string]interface{}{
					"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
				}

				var m map[string]interface{}

				m, ok, err = unstructured.NestedMap(obj.Object, "spec", "template", "spec", "nodeSelector")
				require.NoError(t, err)
				assert.True(t, ok)
				assert.Equal(t, expectedSelector, m)
			}
		})
	}
}

func TestSetVersionNodeAffinity(t *testing.T) {
	for _, kind := range []string{"Pod", "BuildConfig"} {
		t.Run(kind, func(t *testing.T) {
			obj := newObj(kind, "")

			err := SetVersionNodeAffinity(obj, kernelFullVersion)
			require.NoError(t, err)

			expectedSelector := map[string]interface{}{
				"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
			}

			v, ok, err := unstructured.NestedMap(obj.Object, "spec", "nodeSelector")
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, expectedSelector, v)
		})
	}

	for _, kind := range []string{"DaemonSet", "Deployment", "Statefulset"} {
		t.Run(kind, func(t *testing.T) {
			obj := newObj(kind, "")

			err := SetVersionNodeAffinity(obj, kernelFullVersion)

			require.NoError(t, err)

			expectedSelector := map[string]interface{}{
				"feature.node.kubernetes.io/kernel-version.full": kernelFullVersion,
			}

			m, ok, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec", "nodeSelector")
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, expectedSelector, m)
		})
	}

}

func TestIsObjectAffine(t *testing.T) {
	t.Run("not affine", func(t *testing.T) {
		affine := IsObjectAffine(&unstructured.Unstructured{})

		assert.False(t, affine)
	})

	t.Run("affine", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetAnnotations(map[string]string{"specialresource.openshift.io/kernel-affine": "true"})

		affine := IsObjectAffine(obj)

		assert.True(t, affine)
	})
}

func TestPatchVersion(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    kernelFullVersion,
			expected: "4.18.0-305",
		},
		{
			input:    "4.18.0",
			expected: "4.18.0",
		},
		{
			input:    "4.18.0-305",
			expected: "4.18.0-305",
		},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			v, err := PatchVersion(kernelFullVersion)
			require.NoError(t, err)

			assert.Equal(t, "4.18.0-305", v)
		})
	}
}
