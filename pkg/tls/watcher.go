package tls

import commonTLS "github.com/openshift/controller-runtime-common/pkg/tls"

// SecurityProfileWatcher reuses controller-runtime-common's APIServer profile
// and adherence watcher. Use Load for profile-only watching. When setting
// OnAdherencePolicyChange, initialize InitialTLSAdherencePolicy from
// LoadWithAdherence; Load does not read the policy and returns NoOpinion.
type SecurityProfileWatcher = commonTLS.SecurityProfileWatcher
