// Copyright 2016 Palantir Technologies, Inc.
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

package publish_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/palantir/godel/pkg/osarch"
	"github.com/palantir/pkg/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/palantir/distgo/dister"
	"github.com/palantir/distgo/distgo"
	"github.com/palantir/distgo/distgo/dist"
	"github.com/palantir/distgo/distgo/publish"
	"github.com/palantir/distgo/dockerbuilder"
)

const (
	testMain              = `package main; func main(){}`
	testPublisherTypeName = "test-publisher"
)

type testPublisher struct{}

func (p *testPublisher) TypeName() (string, error) {
	return testPublisherTypeName, nil
}

func (p *testPublisher) Flags() ([]distgo.PublisherFlag, error) {
	return nil, nil
}

func (p *testPublisher) RunPublish(productTaskOutputInfo distgo.ProductTaskOutputInfo, cfgYML []byte, flagVals map[distgo.PublisherFlagName]interface{}, dryRun bool, stdout io.Writer) error {
	productDistArtifactPaths := productTaskOutputInfo.ProductDistArtifactPaths()
	var distIDs []distgo.DistID
	for distID := range productDistArtifactPaths {
		distIDs = append(distIDs, distID)
	}
	sort.Sort(distgo.ByDistID(distIDs))

	var outputs []string
	outputs = append(outputs, fmt.Sprintf("Publish the following dist outputs for product %s:", productTaskOutputInfo.Product.ID))
	for _, distID := range distIDs {
		outputs = append(outputs, fmt.Sprintf("%s: %v", distID, productDistArtifactPaths[distID]))
	}
	fmt.Fprintln(stdout, strings.Join(outputs, "\n"))
	return nil
}

func TestPublish(t *testing.T) {
	tmp, cleanup, err := dirs.TempDir("", "")
	defer cleanup()
	require.NoError(t, err)

	for i, tc := range []struct {
		name             string
		projectCfg       distgo.ProjectConfig
		distIDs          []distgo.ProductDistID
		preDistAction    func(projectDir string, projectCfg distgo.ProjectConfig)
		wantStdoutRegexp func(projectDir string) string
	}{
		{
			"publish publishes the dist artifact of a product",
			distgo.ProjectConfig{},
			nil,
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectDir string) string {
				return exactMatchRegexp(fmt.Sprintf(`Publish the following dist outputs for product foo:
os-arch-bin: [%s/out/dist/foo/0.1.0/os-arch-bin/foo-0.1.0-%s.tgz]
`, projectDir, osarch.Current().String()))
			},
		},
		{
			"publish publishes all of the dist artifact of a product",
			distgo.ProjectConfig{
				ProductDefaults: distgo.ProductConfig{
					Build: &distgo.BuildConfig{
						OSArchs: &[]osarch.OSArch{
							mustOSArch("darwin-amd64"),
							mustOSArch("linux-amd64"),
						},
					},
					Dist: &distgo.DistConfig{
						Disters: &distgo.DistersConfig{
							dister.OSArchBinDistTypeName: {
								Type: stringPtr(dister.OSArchBinDistTypeName),
								Config: mustMapSlicePtr(dister.OSArchBinDistConfig{
									OSArchs: []osarch.OSArch{
										mustOSArch("darwin-amd64"),
										mustOSArch("linux-amd64"),
									},
								}),
							},
						},
					},
				},
			},
			nil,
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectDir string) string {
				return exactMatchRegexp(fmt.Sprintf(`Publish the following dist outputs for product foo:
os-arch-bin: [%s/out/dist/foo/0.1.0/os-arch-bin/foo-0.1.0-darwin-amd64.tgz %s/out/dist/foo/0.1.0/os-arch-bin/foo-0.1.0-linux-amd64.tgz]
`, projectDir, projectDir))
			},
		},
		{
			"publish publishes the dist artifact of a product but not its dependencies",
			distgo.ProjectConfig{
				Products: map[distgo.ProductID]distgo.ProductConfig{
					"foo": {
						Build: &distgo.BuildConfig{
							MainPkg: stringPtr("./foo"),
						},
						Dist: &distgo.DistConfig{
							Disters: &distgo.DistersConfig{
								dister.OSArchBinDistTypeName: distgo.DisterConfig{
									Type: stringPtr(dister.OSArchBinDistTypeName),
								},
							},
						},
						Dependencies: &[]distgo.ProductID{
							"bar",
						},
					},
					"bar": {
						Build: &distgo.BuildConfig{
							MainPkg: stringPtr("./foo"),
						},
						Dist: &distgo.DistConfig{
							Disters: &distgo.DistersConfig{
								dister.OSArchBinDistTypeName: distgo.DisterConfig{
									Type: stringPtr(dister.OSArchBinDistTypeName),
								},
							},
						},
					},
				},
			},
			[]distgo.ProductDistID{
				"foo",
			},
			func(projectDir string, projectCfg distgo.ProjectConfig) {
				gittest.CreateGitTag(t, projectDir, "0.1.0")
			},
			func(projectDir string) string {
				return exactMatchRegexp(fmt.Sprintf(`Publish the following dist outputs for product foo:
os-arch-bin: [%s/out/dist/foo/0.1.0/os-arch-bin/foo-0.1.0-%s.tgz]
`, projectDir, osarch.Current().String()))
			},
		},
	} {
		projectDir, err := ioutil.TempDir(tmp, "")
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		gittest.InitGitDir(t, projectDir)
		err = os.MkdirAll(path.Join(projectDir, "foo"), 0755)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		err = ioutil.WriteFile(path.Join(projectDir, "foo", "main.go"), []byte(testMain), 0644)
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		gittest.CommitAllFiles(t, projectDir, "Commit")

		if tc.preDistAction != nil {
			tc.preDistAction(projectDir, tc.projectCfg)
		}

		disterFactory, err := dister.NewDisterFactory()
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		defaultDisterCfg, err := dister.DefaultConfig()
		require.NoError(t, err, "Case %d: %s", i, tc.name)
		dockerBuilderFactory, err := dockerbuilder.NewDockerBuilderFactory()
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		projectParam, err := tc.projectCfg.ToParam(projectDir, disterFactory, defaultDisterCfg, dockerBuilderFactory)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		projectInfo, err := projectParam.ProjectInfo(projectDir)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		buffer := &bytes.Buffer{}
		err = dist.Products(projectInfo, projectParam, nil, false, buffer)
		require.NoError(t, err, "Case %d: %s\nOutput: %s", i, tc.name, buffer.String())

		buffer = &bytes.Buffer{}
		err = publish.Products(projectInfo, projectParam, tc.distIDs, &testPublisher{}, nil, true, buffer)
		require.NoError(t, err, "Case %d: %s", i, tc.name)

		if tc.wantStdoutRegexp != nil {
			assert.Regexp(t, tc.wantStdoutRegexp(projectDir), buffer.String(), "Case %d: %s", i, tc.name)
		}
	}
}

func exactMatchRegexp(in string) string {
	return "^" + regexp.QuoteMeta(in) + "$"
}

func stringPtr(in string) *string {
	return &in
}

func mustMapSlicePtr(in interface{}) *yaml.MapSlice {
	out, err := distgo.ToMapSlice(in)
	if err != nil {
		panic(err)
	}
	return &out
}

func mustOSArch(in string) osarch.OSArch {
	osArch, err := osarch.New(in)
	if err != nil {
		panic(err)
	}
	return osArch
}