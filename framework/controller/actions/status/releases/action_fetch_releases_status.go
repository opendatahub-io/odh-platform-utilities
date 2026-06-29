package releases

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const (
	ComponentMetadataFilename = "component_metadata.yaml"
)

type Action struct {
	fsys                   fs.FS
	metadataFilePathFn     func(rr *types.ReconciliationRequest) string
	componentReleaseStatus common.ComponentReleaseStatus
}

type ActionOpts func(*Action)

func WithFS(fsys fs.FS) ActionOpts {
	return func(a *Action) {
		if fsys != nil {
			a.fsys = fsys
		}
	}
}

func WithMetadataFilePath(fn func(rr *types.ReconciliationRequest) string) ActionOpts {
	return func(a *Action) {
		a.metadataFilePathFn = fn
	}
}

func WithComponentReleaseStatus(status common.ComponentReleaseStatus) ActionOpts {
	return func(a *Action) {
		a.componentReleaseStatus = status
	}
}

func (a *Action) run(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(common.WithReleases)
	if !ok {
		return fmt.Errorf("resource instance %v is not a WithReleases", rr.Instance)
	}

	if len(a.componentReleaseStatus.Releases) == 0 {
		releases, err := a.render(ctx, rr)
		if err != nil {
			return err
		}
		a.componentReleaseStatus = common.ComponentReleaseStatus{Releases: releases}
	}

	obj.SetReleaseStatus(a.componentReleaseStatus)

	return nil
}

func (a *Action) render(ctx context.Context, rr *types.ReconciliationRequest) ([]common.ComponentRelease, error) {
	log := logf.FromContext(ctx)

	metadataPath := a.metadataFilePathFn(rr)

	releases, err := ReadComponentReleases(a.fsys, metadataPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.V(3).Info("Metadata file not found, proceeding with empty releases", "metadataFilePath", metadataPath)
			return nil, nil
		}
		return nil, err
	}

	return releases, nil
}

func NewAction(opts ...ActionOpts) actions.Fn {
	action := Action{
		fsys: os.DirFS("/"),
		metadataFilePathFn: func(rr *types.ReconciliationRequest) string {
			cn := strings.ToLower(rr.Instance.GetObjectKind().GroupVersionKind().Kind)
			mp := filepath.Join(rr.ManifestsBasePath, cn, ComponentMetadataFilename)

			return strings.TrimPrefix(mp, "/")
		},
	}

	for _, opt := range opts {
		opt(&action)
	}

	return action.run
}
