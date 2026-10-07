// Copyright 2026 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package postgres_test

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/postgres"
)

func TestSecretKeyConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "host", got: postgres.SecretKeyHost, want: "host"},
		{name: "port", got: postgres.SecretKeyPort, want: "port"},
		{name: "user", got: postgres.SecretKeyUser, want: "user"},
		{name: "password", got: postgres.SecretKeyPassword, want: "password"},
		{name: "database", got: postgres.SecretKeyDatabase, want: "dbname"},
		{name: "schema", got: postgres.SecretKeySchema, want: "schema"},
		{name: "sslmode", got: postgres.SecretKeySSLMode, want: "sslmode"},
		{name: "ca", got: postgres.SecretKeyCA, want: "ca.crt"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
