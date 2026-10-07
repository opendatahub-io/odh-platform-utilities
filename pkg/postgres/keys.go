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

// Package postgres defines the Secret key names for a PostgreSQL connection-
// credentials Secret. These names are part of this project's database-
// connection-surface ADR. The keys are undotted so Kubernetes can safely
// project the Secret with envFrom: EnvFromSource silently skips keys that
// contain a period. The ca.crt key is the deliberate exception because it is
// consumed as a mounted file, not as an environment variable, matching the
// kube-root-ca.crt and CNPG convention.
package postgres

// These undotted keys are required by the database-connection-surface ADR for
// PostgreSQL connection credentials. Keeping them free of periods allows the
// Secret to be projected with Kubernetes envFrom, which silently drops keys
// containing a period.
const (
	// SecretKeyHost is the Secret key for the database host.
	SecretKeyHost = "host"
	// SecretKeyPort is the Secret key for the database port.
	SecretKeyPort = "port"
	// SecretKeyUser is the Secret key for the database user.
	SecretKeyUser = "user"
	// SecretKeyPassword is the Secret key for the database password.
	SecretKeyPassword = "password"
	// SecretKeyDatabase is the Secret key for the database name.
	SecretKeyDatabase = "dbname"
	// SecretKeySchema is the Secret key for the database schema.
	SecretKeySchema = "schema"
	// SecretKeySSLMode is the Secret key for the PostgreSQL SSL mode.
	SecretKeySSLMode = "sslmode"
)

// SecretKeyCA is the Secret key for the CA certificate. It is the deliberate
// dotted-key exception because it is consumed as a mounted file, not an
// environment variable, matching the kube-root-ca.crt and CNPG convention.
const SecretKeyCA = "ca.crt"
