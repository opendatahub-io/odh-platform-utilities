package resources

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
)

// ListAvailableAPIResources returns all preferred API resources from the
// cluster. Group discovery errors (e.g., aggregated APIs that require special
// authentication) are silently swallowed so that GC can proceed with the
// groups that are available.
func ListAvailableAPIResources(cli discovery.DiscoveryInterface) ([]*metav1.APIResourceList, error) {
	items, err := cli.ServerPreferredResources()

	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, fmt.Errorf("failure retrieving supported resources: %w", err)
	}

	return items, nil
}
