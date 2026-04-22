package cluster_test

import (
	"context"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	return s
}

func TestGetSingleton_ZeroInstances(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	target := &corev1.ConfigMap{}

	err := cluster.GetSingleton(context.Background(), c, target)
	require.Error(t, err)
	assert.ErrorIs(t, err, cluster.ErrNoInstance)
}

func TestGetSingleton_OneInstance(t *testing.T) {
	t.Parallel()

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "singleton-cm",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(existing).
		Build()

	target := &corev1.ConfigMap{}
	err := cluster.GetSingleton(context.Background(), c, target)

	require.NoError(t, err)
	assert.Equal(t, "singleton-cm", target.Name)
	assert.Equal(t, "default", target.Namespace)
	assert.Equal(t, "value", target.Data["key"])
}

func TestGetSingleton_MultipleInstances(t *testing.T) {
	t.Parallel()

	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "default"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-2", Namespace: "default"},
	}

	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cm1, cm2).
		Build()

	target := &corev1.ConfigMap{}
	err := cluster.GetSingleton(context.Background(), c, target)

	require.Error(t, err)
	assert.ErrorIs(t, err, cluster.ErrMultipleInstances)
}

func TestGetSingleton_UnregisteredType(t *testing.T) {
	t.Parallel()

	emptyScheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(emptyScheme).Build()
	target := &corev1.ConfigMap{}

	err := cluster.GetSingleton(context.Background(), c, target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "determining GVK")
	require.ErrorIs(t, err, cluster.ErrUnregisteredType)
	assert.NotErrorIs(t, err, cluster.ErrNoInstance,
		"scheme lookup failure must not match ErrNoInstance")
}
