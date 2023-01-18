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

package model

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model/pretty"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
)

// ComponentType represents component.
type ComponentType struct {
	// Name is the name of the component type.
	Name string
}

// NewComponentType creates a new component type with the given name.
func NewComponentType(name string) *ComponentType {
	return &ComponentType{Name: name}
}

// SyntaxNode returns the syntax node for the type. This is always syntax.None.
func (*ComponentType) SyntaxNode() hclsyntax.Node {
	return syntax.None
}

func (t *ComponentType) Pretty() pretty.Formatter {
	return &pretty.Wrap{
		Prefix:  "component(",
		Postfix: ")",
		Value:   pretty.FromString(t.Name),
	}
}

// Traverse attempts to traverse the optional type with the given traverser. The result type of traverse(list(T))
// is T; the traversal fails if the traverser is not a number.
func (t *ComponentType) Traverse(traverser hcl.Traverser) (Traversable, hcl.Diagnostics) {
	_, indexType := GetTraverserKey(traverser)

	var diagnostics hcl.Diagnostics
	if !InputType(NumberType).ConversionFrom(indexType).Exists() {
		diagnostics = hcl.Diagnostics{unsupportedListIndex(traverser.SourceRange())}
	}
	return t.ElementType, diagnostics
}

// Equals returns true if this type has the same identity as the given type.
func (t *ComponentType) Equals(other Type) bool {
	return t.equals(other, nil)
}

func (t *ComponentType) equals(other Type, seen map[Type]struct{}) bool {
	if t == other {
		return true
	}

	otherComponent, ok := other.(*ComponentType)
	return ok && t.Name == otherComponent.Name
}

// AssignableFrom returns true if this type is assignable from the indicated source type. A list(T) is assignable
// from values of type list(U) where T is assignable from U.
func (t *ComponentType) AssignableFrom(src Type) bool {
	switch src := src.(type) {
	case *ComponentType:
		if src.Name == t.Name {
			return true
		}
	}
	return false
}

// ConversionFrom returns the kind of conversion (if any) that is possible from the source type to this type. A list(T)
// is safely convertible from list(U), set(U), or tuple(U_0 ... U_N) if the element type(s) U is/are safely convertible
// to T. If any element type is unsafely convertible to T and no element type is safely convertible to T, the
// conversion is unsafe. Otherwise, no conversion exists.
func (t *ComponentType) ConversionFrom(src Type) ConversionKind {
	kind, _ := t.conversionFrom(src, false, nil)
	return kind
}

func (t *ComponentType) conversionFrom(src Type, unifying bool, seen map[Type]struct{}) (ConversionKind, lazyDiagnostics) {
	switch src := src.(type) {
	case *ComponentType:
		if src.Name == t.Name {
			return SafeConversion, nil
		}
	}
	return NoConversion, func() hcl.Diagnostics { return hcl.Diagnostics{typeNotConvertible(t, src)} }
}

func (t *ComponentType) String() string {
	return t.string(nil)
}

func (t *ComponentType) string(seen map[Type]struct{}) string {
	return fmt.Sprintf("component(%s)", t.Name)
}

func (t *ComponentType) unify(other Type) (Type, ConversionKind) {
	return unify(t, other, func() (Type, ConversionKind) {
		switch other := other.(type) {
		case *TupleType:
			// If the other element is a list type, prefer the list type, but unify the element type.
			elementType, conversionKind := t.ElementType, SafeConversion
			for _, other := range other.ElementTypes {
				element, ck := elementType.unify(other)
				if ck < conversionKind {
					conversionKind = ck
				}
				elementType = element
			}
			return NewListType(elementType), conversionKind
		case *SetType:
			// If the other element is a set type, prefer the list type, but unify the element types.
			elementType, conversionKind := t.ElementType.unify(other.ElementType)
			return NewListType(elementType), conversionKind
		case *ListType:
			// If the other type is a list type, unify based on the element type.
			elementType, conversionKind := t.ElementType.unify(other.ElementType)
			return NewListType(elementType), conversionKind
		default:
			// Prefer the list type.
			kind, _ := t.conversionFrom(other, true, nil)
			return t, kind
		}
	})
}

func (*ComponentType) isType() {}
