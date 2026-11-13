package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHasSharedAssociativeKey verifies the shared-associative-key relation
// over managed-fields fieldsets used to detect release-pending owners: two
// managers claim the same associative-list entry ("k:<merge-keys>" key) or
// set member ("v:<value>" key) at the same path of regular fields. In
// fieldsV1 JSON, associative-list entries appear as "k:" keys with the
// merge keys as escaped JSON inside the key.
func TestHasSharedAssociativeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "regular fields alone share nothing",
			a:    `{"f:spec":{"f:replicas":{}}}`,
			b:    `{"f:spec":{"f:replicas":{},"f:paused":{}}}`,
			want: false,
		},
		{
			name: "labels alone share nothing",
			a:    `{"f:metadata":{"f:labels":{"f:app":{}}}}`,
			b:    `{"f:metadata":{"f:labels":{"f:app":{},"f:other":{}}}}`,
			want: false,
		},
		{
			name: "the same associative-list entry is shared",
			a:    `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{"f:image":{}}}}}}}`,
			b:    `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{"f:name":{}}}}}}}`,
			want: true,
		},
		{
			name: "different associative-list entries share nothing",
			a:    `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{"f:ports":{"k:{\"containerPort\":8080,\"protocol\":\"TCP\"}":{}}}}}}}}`,
			b:    `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"sidecar\"}":{"f:ports":{"k:{\"containerPort\":8080,\"protocol\":\"TCP\"}":{}}}}}}}}`,
			want: false,
		},
		{
			name: "the same container entry with different subfields is shared",
			a:    `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{"f:ports":{"k:{\"containerPort\":8080,\"protocol\":\"TCP\"}":{}}}}}}}}`,
			b:    `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{"f:ports":{"k:{\"containerPort\":8443,\"protocol\":\"TCP\"}":{}}}}}}}}`,
			want: true,
		},
		{
			name: "associative-list entries under different paths share nothing",
			a:    `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{}}}}}}`,
			b:    `{"f:spec":{"f:template":{"f:spec":{"f:initContainers":{"k:{\"name\":\"manager\"}":{}}}}}}`,
			want: false,
		},
		{
			name: "the same set member is shared",
			a:    `{"f:spec":{"f:tolerations":{"v:\"key=foo,effect=NoSchedule\"":{}}}}`,
			b:    `{"f:spec":{"f:tolerations":{"v:\"key=foo,effect=NoSchedule\"":{},"v:\"key=bar\"":{}}}}`,
			want: true,
		},
		{
			name: "different set members share nothing",
			a:    `{"f:spec":{"f:tolerations":{"v:\"key=foo,effect=NoSchedule\"":{}}}}`,
			b:    `{"f:spec":{"f:tolerations":{"v:\"key=bar,effect=NoSchedule\"":{}}}}`,
			want: false,
		},
		{
			name: "a shared entry deep under different regular fields is not shared",
			a:    `{"f:metadata":{"f:annotations":{"k:{\"name\":\"x\"}":{}}}}`,
			b:    `{"f:spec":{"f:template":{"f:metadata":{"f:annotations":{"k:{\"name\":\"x\"}":{}}}}}}`,
			want: false,
		},
		{
			name: "malformed a shares nothing",
			a:    `{invalid`,
			b:    `{"f:spec":{}}`,
			want: false,
		},
		{
			name: "malformed b shares nothing",
			a:    `{"f:spec":{}}`,
			b:    `{invalid`,
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, hasSharedAssociativeKey([]byte(test.a), []byte(test.b)))
		})
	}
}

// TestConvergedGuardianFieldset verifies the converged-guardian detection
// over managed-fields fieldsets: a previous owner whose claims a preserving
// release reduced to spec.replicas and the merge key and resources of the
// containers the desired state still carries is the owner of record for
// the Deployment user-tuned fields and must not be migrated again. Anything
// else — other fields, or claims for containers the desired state dropped —
// is stale and must be handed off.
func TestConvergedGuardianFieldset(t *testing.T) {
	t.Parallel()

	desired := map[string]struct{}{"manager": {}}

	tests := []struct {
		name   string
		fields string
		want   bool
	}{
		{
			name:   "the preserving release result is converged",
			fields: `{"f:spec":{"f:replicas":{},"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{".":{},"f:name":{},"f:resources":{"f:requests":{"f:cpu":{}}}}}}}}}`,
			want:   true,
		},
		{
			name:   "replicas alone is converged",
			fields: `{"f:spec":{"f:replicas":{}}}`,
			want:   true,
		},
		{
			name:   "container skeletons alone are converged",
			fields: `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{".":{},"f:name":{}}}}}}}`,
			want:   true,
		},
		{
			name:   "an empty fieldset is converged",
			fields: `{}`,
			want:   true,
		},
		{
			name:   "claims outside spec are stale",
			fields: `{"f:metadata":{"f:labels":{"f:app":{}}},"f:spec":{"f:replicas":{}}}`,
			want:   false,
		},
		{
			name:   "claims under spec beyond replicas and the template are stale",
			fields: `{"f:spec":{"f:paused":{},"f:replicas":{}}}`,
			want:   false,
		},
		{
			name:   "container claims beyond the merge key and resources are stale",
			fields: `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{".":{},"f:image":{},"f:name":{}}}}}}}`,
			want:   false,
		},
		{
			name:   "stale port entries are stale",
			fields: `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"manager\"}":{".":{},"f:name":{},"f:ports":{"k:{\"containerPort\":8080,\"protocol\":\"TCP\"}":{}}}}}}}}`,
			want:   false,
		},
		{
			name:   "claims for a container the desired state dropped are stale",
			fields: `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"removed\"}":{".":{},"f:name":{},"f:resources":{}}}}}}}`,
			want:   false,
		},
		{
			name:   "a non-keyed containers claim is stale",
			fields: `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"v:\"manager\"":{}}}}}}`,
			want:   false,
		},
		{
			name:   "a malformed fieldset is stale",
			fields: `{invalid`,
			want:   false,
		},
		{
			name:   "a missing fieldset is stale",
			fields: ``,
			want:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, convergedGuardianFieldset([]byte(test.fields), desired))
		})
	}
}
