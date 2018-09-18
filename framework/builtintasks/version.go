/*
Sniperkit-Bot
- Status: analyzed
*/

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

package builtintasks

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sniperkit/snk.fork.palantir-godel/framework/godel"
	"github.com/sniperkit/snk.fork.palantir-godel/framework/godellauncher"
)

var Version = "unspecified"

func VersionTask() godellauncher.Task {
	return godellauncher.CobraCLITask(&cobra.Command{
		Use:   "version",
		Short: fmt.Sprintf("Print %s version", godel.AppName),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), godel.VersionOutput())
			return nil
		},
	}, nil)
}
