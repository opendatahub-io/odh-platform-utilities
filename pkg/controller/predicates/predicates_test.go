package predicates_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/predicates"
)

func obj(generation int64, lbls, annotations map[string]string) client.Object {
	o := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Generation:  generation,
			Labels:      lbls,
			Annotations: annotations,
		},
	}

	return o
}

// --- GenerationChangedPredicate ---

func TestGenerationChangedPredicate_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ predicate.Predicate = predicates.GenerationChangedPredicate{}
}

func TestGenerationChangedPredicate_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		oldGen   int64
		newGen   int64
		expected bool
	}{
		{name: "same generation", oldGen: 1, newGen: 1, expected: false},
		{name: "generation incremented", oldGen: 1, newGen: 2, expected: true},
		{name: "generation decremented", oldGen: 3, newGen: 1, expected: true},
		{name: "old generation zero", oldGen: 0, newGen: 5, expected: true},
		{name: "new generation zero", oldGen: 5, newGen: 0, expected: true},
		{name: "both generation zero", oldGen: 0, newGen: 0, expected: true},
	}

	p := predicates.GenerationChangedPredicate{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := p.Update(event.UpdateEvent{
				ObjectOld: obj(tt.oldGen, nil, nil),
				ObjectNew: obj(tt.newGen, nil, nil),
			})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestGenerationChangedPredicate_Update_NilObjects(t *testing.T) {
	t.Parallel()

	p := predicates.GenerationChangedPredicate{}

	tests := []struct {
		e    event.UpdateEvent
		name string
	}{
		{name: "nil old", e: event.UpdateEvent{ObjectOld: nil, ObjectNew: obj(1, nil, nil)}},
		{name: "nil new", e: event.UpdateEvent{ObjectOld: obj(1, nil, nil), ObjectNew: nil}},
		{name: "both nil", e: event.UpdateEvent{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.False(t, p.Update(tt.e))
		})
	}
}

func TestGenerationChangedPredicate_PassthroughEvents(t *testing.T) {
	t.Parallel()

	p := predicates.GenerationChangedPredicate{}

	assert.True(t, p.Create(event.CreateEvent{Object: obj(1, nil, nil)}),
		"create events should pass through")
	assert.True(t, p.Delete(event.DeleteEvent{Object: obj(1, nil, nil)}),
		"delete events should pass through")
	assert.True(t, p.Generic(event.GenericEvent{Object: obj(1, nil, nil)}),
		"generic events should pass through")
}

// --- LabelSelectorPredicate ---

func TestLabelSelectorPredicate_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ predicate.Predicate = predicates.LabelSelectorPredicate{}
}

func TestLabelSelectorPredicate_Create(t *testing.T) {
	t.Parallel()

	sel := labels.SelectorFromSet(labels.Set{"app": "test"})

	tests := []struct {
		labels   map[string]string
		name     string
		expected bool
	}{
		{name: "matching labels", labels: map[string]string{"app": "test"}, expected: true},
		{name: "extra labels match", labels: map[string]string{"app": "test", "env": "prod"}, expected: true},
		{name: "wrong value", labels: map[string]string{"app": "other"}, expected: false},
		{name: "missing label", labels: map[string]string{"env": "prod"}, expected: false},
		{name: "nil labels", labels: nil, expected: false},
	}

	p := predicates.LabelSelectorPredicate{Selector: sel}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := p.Create(event.CreateEvent{Object: obj(0, tt.labels, nil)})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestLabelSelectorPredicate_Update(t *testing.T) {
	t.Parallel()

	sel := labels.SelectorFromSet(labels.Set{"app": "test"})
	p := predicates.LabelSelectorPredicate{Selector: sel}

	t.Run("new object matches", func(t *testing.T) {
		t.Parallel()

		got := p.Update(event.UpdateEvent{
			ObjectOld: obj(0, nil, nil),
			ObjectNew: obj(0, map[string]string{"app": "test"}, nil),
		})
		assert.True(t, got)
	})

	t.Run("new object does not match", func(t *testing.T) {
		t.Parallel()

		got := p.Update(event.UpdateEvent{
			ObjectOld: obj(0, map[string]string{"app": "test"}, nil),
			ObjectNew: obj(0, map[string]string{"app": "other"}, nil),
		})
		assert.False(t, got)
	})

	t.Run("nil new object", func(t *testing.T) {
		t.Parallel()

		got := p.Update(event.UpdateEvent{
			ObjectOld: obj(0, nil, nil),
			ObjectNew: nil,
		})
		assert.False(t, got)
	})
}

func TestLabelSelectorPredicate_Delete(t *testing.T) {
	t.Parallel()

	sel := labels.SelectorFromSet(labels.Set{"app": "test"})
	p := predicates.LabelSelectorPredicate{Selector: sel}

	assert.True(t, p.Delete(event.DeleteEvent{
		Object: obj(0, map[string]string{"app": "test"}, nil),
	}))
	assert.False(t, p.Delete(event.DeleteEvent{
		Object: obj(0, map[string]string{"app": "other"}, nil),
	}))
}

func TestLabelSelectorPredicate_Generic(t *testing.T) {
	t.Parallel()

	sel := labels.SelectorFromSet(labels.Set{"app": "test"})
	p := predicates.LabelSelectorPredicate{Selector: sel}

	assert.True(t, p.Generic(event.GenericEvent{
		Object: obj(0, map[string]string{"app": "test"}, nil),
	}))
	assert.False(t, p.Generic(event.GenericEvent{
		Object: obj(0, nil, nil),
	}))
}

func TestLabelSelectorPredicate_NilSelector(t *testing.T) {
	t.Parallel()

	p := predicates.LabelSelectorPredicate{}

	assert.True(t, p.Create(event.CreateEvent{Object: obj(0, nil, nil)}),
		"nil selector should match everything")
	assert.True(t, p.Update(event.UpdateEvent{
		ObjectOld: obj(0, nil, nil),
		ObjectNew: obj(0, nil, nil),
	}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: obj(0, nil, nil)}))
	assert.True(t, p.Generic(event.GenericEvent{Object: obj(0, nil, nil)}))
}

func TestLabelSelectorPredicate_EmptySelector(t *testing.T) {
	t.Parallel()

	p := predicates.LabelSelectorPredicate{
		Selector: labels.Everything(),
	}

	assert.True(t, p.Create(event.CreateEvent{Object: obj(0, nil, nil)}),
		"empty selector should match everything")
}

func TestLabelSelectorPredicate_NilObjects(t *testing.T) {
	t.Parallel()

	sel := labels.SelectorFromSet(labels.Set{"app": "test"})
	p := predicates.LabelSelectorPredicate{Selector: sel}

	assert.False(t, p.Create(event.CreateEvent{Object: nil}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: nil}))
	assert.False(t, p.Generic(event.GenericEvent{Object: nil}))
}

// --- AnnotationChangedPredicate ---

func TestAnnotationChangedPredicate_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ predicate.Predicate = predicates.AnnotationChangedPredicate{}
}

func TestAnnotationChangedPredicate_Create(t *testing.T) {
	t.Parallel()

	p := predicates.AnnotationChangedPredicate{Key: "my-annotation"}

	assert.True(t, p.Create(event.CreateEvent{Object: obj(0, nil, nil)}),
		"create events should always pass")
	assert.True(t, p.Create(event.CreateEvent{
		Object: obj(0, nil, map[string]string{"my-annotation": "v1"}),
	}))
}

func TestAnnotationChangedPredicate_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		oldAnnot map[string]string
		newAnnot map[string]string
		name     string
		expected bool
	}{
		{
			name:     "annotation value changed",
			oldAnnot: map[string]string{"config-hash": "abc"},
			newAnnot: map[string]string{"config-hash": "def"},
			expected: true,
		},
		{
			name:     "annotation value unchanged",
			oldAnnot: map[string]string{"config-hash": "abc"},
			newAnnot: map[string]string{"config-hash": "abc"},
			expected: false,
		},
		{
			name:     "annotation added",
			oldAnnot: nil,
			newAnnot: map[string]string{"config-hash": "abc"},
			expected: true,
		},
		{
			name:     "annotation removed",
			oldAnnot: map[string]string{"config-hash": "abc"},
			newAnnot: nil,
			expected: true,
		},
		{
			name:     "annotation absent on both",
			oldAnnot: nil,
			newAnnot: nil,
			expected: false,
		},
		{
			name:     "unrelated annotation changed",
			oldAnnot: map[string]string{"other": "a"},
			newAnnot: map[string]string{"other": "b"},
			expected: false,
		},
	}

	p := predicates.AnnotationChangedPredicate{Key: "config-hash"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := p.Update(event.UpdateEvent{
				ObjectOld: obj(1, nil, tt.oldAnnot),
				ObjectNew: obj(1, nil, tt.newAnnot),
			})
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestAnnotationChangedPredicate_Update_NilObjects(t *testing.T) {
	t.Parallel()

	p := predicates.AnnotationChangedPredicate{Key: "config-hash"}

	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: nil, ObjectNew: obj(1, nil, nil)}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: obj(1, nil, nil), ObjectNew: nil}))
	assert.False(t, p.Update(event.UpdateEvent{}))
}

func TestAnnotationChangedPredicate_DeleteAndGeneric(t *testing.T) {
	t.Parallel()

	p := predicates.AnnotationChangedPredicate{Key: "config-hash"}

	assert.False(t, p.Delete(event.DeleteEvent{Object: obj(0, nil, nil)}),
		"delete events should be rejected")
	assert.False(t, p.Generic(event.GenericEvent{Object: obj(0, nil, nil)}),
		"generic events should be rejected")
}

// --- DeletionPredicate ---

func TestDeletionPredicate_ImplementsInterface(t *testing.T) {
	t.Parallel()

	var _ predicate.Predicate = predicates.DeletionPredicate{}
}

func TestDeletionPredicate_EventFiltering(t *testing.T) {
	t.Parallel()

	p := predicates.DeletionPredicate{}
	o := obj(1, nil, nil)

	assert.False(t, p.Create(event.CreateEvent{Object: o}), "create should be rejected")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: o, ObjectNew: o}), "update should be rejected")
	assert.True(t, p.Delete(event.DeleteEvent{Object: o}), "delete should pass")
	assert.False(t, p.Generic(event.GenericEvent{Object: o}), "generic should be rejected")
}
