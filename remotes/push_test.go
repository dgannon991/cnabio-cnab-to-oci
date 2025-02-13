package remotes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cnabio/cnab-go/bundle"
	"github.com/cnabio/cnab-to-oci/converter"
	"github.com/cnabio/cnab-to-oci/tests"
	"github.com/distribution/reference"
	ocischemav1 "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

const (
	expectedBundleManifest = `{
  "schemaVersion": 2,
  "manifests": [
    {
      "mediaType":"application/vnd.oci.image.manifest.v1+json",
      "digest":"sha256:122a5dc186ec285488de9d25e99c96a11d3f7ff71c6e05a06c98e8627472a920",
      "size":189,
      "annotations":{
        "io.cnab.manifest.type":"config"
      }
    },
    {
      "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
      "digest": "sha256:d59a1aa7866258751a261bae525a1842c7ff0662d4f34a355d5f36826abc0343",
      "size": 506,
      "annotations": {
        "io.cnab.manifest.type": "invocation"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:d59a1aa7866258751a261bae525a1842c7ff0662d4f34a355d5f36826abc0342",
      "size": 507,
      "annotations": {
        "io.cnab.component.name": "another-image",
        "io.cnab.manifest.type": "component"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:d59a1aa7866258751a261bae525a1842c7ff0662d4f34a355d5f36826abc0341",
      "size": 507,
      "annotations": {
        "io.cnab.component.name": "image-1",
        "io.cnab.manifest.type": "component"
      }
    }
  ],
  "annotations": {
    "io.cnab.keywords": "[\"keyword1\",\"keyword2\"]",
    "io.cnab.runtime_version": "v1.0.0",
    "org.opencontainers.artifactType": "application/vnd.cnab.manifest.v1",
    "org.opencontainers.image.authors": "[{\"name\":\"docker\",\"email\":\"docker@docker.com\",\"url\":\"docker.com\"}]",
    "org.opencontainers.image.description": "description",
    "org.opencontainers.image.title": "my-app",
    "org.opencontainers.image.version": "0.1.0"
  }
}`
	expectedConfigManifest = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "size": 1596,
      "digest": "sha256:dbe3480b9cb300f389e8d02e4a682f9107772468feb6845f912dc8deed6d76fd"
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.container.image.v1+json",
         "size": 1596,
         "digest": "sha256:dbe3480b9cb300f389e8d02e4a682f9107772468feb6845f912dc8deed6d76fd"
      }
   ]
}`
)

func TestPush(t *testing.T) {
	pusher := &mockPusher{}
	resolver := &mockResolver{pusher: pusher}
	b := tests.MakeTestBundle()
	expectedBundleConfig, err := b.Marshal()
	assert.NilError(t, err, "marshaling to canonical json failed")
	ref, err := reference.ParseNamed("my.registry/namespace/my-app:my-tag")
	assert.NilError(t, err, "parsing the OCI reference failed")

	// push the bundle
	descriptor, err := Push(context.Background(), b, tests.MakeRelocationMap(), ref, resolver, true)
	assert.NilError(t, err, "push failed")
	assert.Equal(t, tests.BundleDigest, descriptor.Digest)
	assert.Equal(t, len(resolver.pushedReferences), 3)
	assert.Equal(t, len(pusher.pushedDescriptors), 3)
	assert.Equal(t, len(pusher.buffers), 3)

	// check pushed config
	assert.Equal(t, "my.registry/namespace/my-app", resolver.pushedReferences[0])
	assert.Equal(t, converter.CNABConfigMediaType, pusher.pushedDescriptors[0].MediaType)
	assert.Equal(t, oneLiner(string(expectedBundleConfig)), pusher.buffers[0].String())

	// check pushed config manifest
	assert.Equal(t, "my.registry/namespace/my-app", resolver.pushedReferences[1])
	assert.Equal(t, ocischemav1.MediaTypeImageManifest, pusher.pushedDescriptors[1].MediaType)

	// check pushed bundle manifest index
	assert.Equal(t, "my.registry/namespace/my-app:my-tag", resolver.pushedReferences[2])
	assert.Equal(t, ocischemav1.MediaTypeImageIndex, pusher.pushedDescriptors[2].MediaType)
	assert.Equal(t, oneLiner(expectedBundleManifest), pusher.buffers[2].String())
}

func TestFallbackConfigManifest(t *testing.T) {
	// Make the pusher return an error for the first two calls
	// so that the fallbacks kick in and we get the non-oci
	// config manifest.
	pusher := newMockPusher([]error{
		errors.New("1"),
		errors.New("2"),
		nil,
		nil,
		nil,
		nil,
		nil})
	resolver := &mockResolver{pusher: pusher}
	b := tests.MakeTestBundle()
	ref, err := reference.ParseNamed("my.registry/namespace/my-app:my-tag")
	assert.NilError(t, err)

	// push the bundle
	relocationMap := tests.MakeRelocationMap()
	_, err = Push(context.Background(), b, relocationMap, ref, resolver, true)
	assert.NilError(t, err)
	assert.Equal(t, expectedConfigManifest, pusher.buffers[3].String())
}

func oneLiner(s string) string {
	return strings.Replace(strings.Replace(s, " ", "", -1), "\n", "", -1)
}

func ExamplePush() {
	resolver := createExampleResolver()
	b := createExampleBundle()
	ref, err := reference.ParseNamed("my.registry/namespace/my-app:my-tag")
	if err != nil {
		panic(err)
	}

	// Push the bundle here
	descriptor, err := Push(context.Background(), b, tests.MakeRelocationMap(), ref, resolver, true)
	if err != nil {
		panic(err)
	}

	bytes, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", string(bytes))

	// Output:
	// {
	//   "mediaType": "application/vnd.oci.image.index.v1+json",
	//   "digest": "sha256:00043ca96c3c9470fc0f647c67613ddf500941556d1ecc14d75bc9b2834f66c3",
	//   "size": 1360
	// }
}

func createExampleBundle() *bundle.Bundle {
	return tests.MakeTestBundle()
}
