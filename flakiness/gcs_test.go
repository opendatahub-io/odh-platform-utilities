package flakiness_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

type fakeBucketClient struct {
	objects map[string]map[string][]byte // bucket -> name -> content
	listErr error
	readErr error
}

func newFakeBucketClient() *fakeBucketClient {
	return &fakeBucketClient{
		objects: make(map[string]map[string][]byte),
	}
}

func (f *fakeBucketClient) addObject(bucket, name string, content []byte) {
	if f.objects[bucket] == nil {
		f.objects[bucket] = make(map[string][]byte)
	}

	f.objects[bucket][name] = content
}

func (f *fakeBucketClient) ListObjects(_ context.Context, bucket, prefix string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	var names []string

	for name := range f.objects[bucket] {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			names = append(names, name)
		}
	}

	return names, nil
}

func (f *fakeBucketClient) ReadObject(_ context.Context, bucket, name string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}

	bucketObjs, ok := f.objects[bucket]
	if !ok {
		return nil, fmt.Errorf("bucket %q not found", bucket)
	}

	data, ok := bucketObjs[name]
	if !ok {
		return nil, fmt.Errorf("object %q not found in bucket %q", name, bucket)
	}

	return data, nil
}

var _ flakiness.BucketClient = (*fakeBucketClient)(nil)

func TestFakeBucketClient_ListObjects(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("test-bucket", "logs/job-a/1/junit.xml", []byte("<xml/>"))
	client.addObject("test-bucket", "logs/job-a/2/junit.xml", []byte("<xml/>"))
	client.addObject("test-bucket", "logs/job-b/1/junit.xml", []byte("<xml/>"))

	names, err := client.ListObjects(context.Background(), "test-bucket", "logs/job-a/")
	require.NoError(t, err)
	assert.Len(t, names, 2)
}

func TestFakeBucketClient_ReadObject(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket", "path/file.xml", []byte("content"))

	data, err := client.ReadObject(context.Background(), "bucket", "path/file.xml")
	require.NoError(t, err)
	assert.Equal(t, []byte("content"), data)
}

func TestFakeBucketClient_ReadObject_NotFound(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()

	_, err := client.ReadObject(context.Background(), "bucket", "missing")
	require.Error(t, err)
}
