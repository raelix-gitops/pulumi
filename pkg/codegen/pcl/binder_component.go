// Copyright 2016-2020, Pulumi Corporation.
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

//nolint:goconst
package pcl

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
)

func (b *binder) bindComponent(node *Component) hcl.Diagnostics {
	block, diagnostics := model.BindBlock(node.syntax, model.StaticScope(b.root), b.tokens, b.options.modelOptions()...)
	node.Definition = block

	if sourceAttr, ok := block.Body.Attribute("source"); ok {
		source, lDiags := getStringAttrValue(sourceAttr)
		if lDiags != nil {
			diagnostics = diagnostics.Append(lDiags)
			return diagnostics
		} else {
			node.source = source
		}
	}

	// check we can use components and load the program
	if b.options.componentLoader == nil {
		diagnostics = diagnostics.Append(errorf(node.Syntax.Expr.Range(), "components are not supported"))
		return diagnostics
	}

	program = b.options.componentLoader(node.source)

	return diagnostics
}
