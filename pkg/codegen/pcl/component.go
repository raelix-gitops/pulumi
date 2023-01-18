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

package pcl

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
)

// Component represents a component reference in a program.
type Component struct {
	node

	syntax *hclsyntax.Block

	// The name visible to API calls related to the component. Used as the Name argument in component
	// constructors, and through those calls to RegisterResource. Must not be modified during code
	// generation to ensure that components are not renamed (deleted and recreated).
	name string

	// The location of the source for the component.
	source string

	// The inner Program that makes up this Component.
	program *Program

	// The type of the resource variable.
	VariableType model.Type

	// The definition of the component.
	Definition *model.Block

	// The component's input attributes, in source order.
	Inputs []*model.Attribute
}

// SyntaxNode returns the syntax node associated with the component.
func (c *Component) SyntaxNode() hclsyntax.Node {
	return c.syntax
}

func (c *Component) Name() string {
	return c.name
}

func (c *Component) Type() model.Type {
	panic("TODO")
}

func (c *Component) Traverse(traverser hcl.Traverser) (model.Traversable, hcl.Diagnostics) {
	panic("TODO")
}

func (c *Component) VisitExpressions(pre, post model.ExpressionVisitor) hcl.Diagnostics {
	return model.VisitExpressions(r.Definition, pre, post)
}
