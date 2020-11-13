package translate

import (
	"github.com/crossplane-contrib/terraform-provider-gen/pkg/generator"
	"github.com/hashicorp/terraform/configs/configschema"
	"github.com/hashicorp/terraform/providers"
	"github.com/iancoleman/strcase"
	"github.com/zclconf/go-cty/cty"
)

type SpecOrStatusField int

const (
	ForProviderField SpecOrStatusField = iota
	AtProviderField
)

type FieldBuilder struct {
	f *generator.Field
}

func NewFieldBuilder(name string) *FieldBuilder {
	fb := &FieldBuilder{f: &generator.Field{}}
	return fb.Name(name)
}

func (fb *FieldBuilder) Name(name string) *FieldBuilder {
	fb.f.Name = strcase.ToCamel(name)
	fb.f.Tag = &generator.StructTag{
		&generator.StructTagJson{
			Name: name,
		},
	}
	return fb
}

func (fb *FieldBuilder) AttributeField(af generator.AttributeField) *FieldBuilder {
	fb.f.Type = generator.FieldTypeAttribute
	fb.f.AttributeField = af
	return fb
}

func (fb *FieldBuilder) StructField(typeName string, fields []generator.Field) *FieldBuilder {
	fb.f.Type = generator.FieldTypeStruct
	// do we need to plumb through package path? that whole concept is getting tiresome
	// and isn't used for anything so far
	fb.f.StructField = generator.StructField{
		TypeName: typeName,
	}
	fb.f.Fields = fields
	return fb
}

func (fb *FieldBuilder) IsSlice(is bool) *FieldBuilder {
	fb.f.IsSlice = is
	return fb
}

func (fb *FieldBuilder) Unsupported() generator.Field {
	return fb.AttributeField(
		generator.AttributeField{Type: generator.AttributeTypeUnsupported}).Build()
}

func (fb *FieldBuilder) ObjectField(typeName string, attrType cty.Type) *FieldBuilder {
	fields := make([]generator.Field, 0)
	for k, t := range attrType.ElementType().AttributeTypes() {
		fields = append(fields, TypeToField(k, t))
	}
	return fb.StructField(typeName, fields)
}

func (fb *FieldBuilder) Build() generator.Field {
	return *fb.f
}

// TypeToField converts a terraform *configschema.Attribute
// to a crossplane generator.Field
func TypeToField(name string, attrType cty.Type) generator.Field {
	fb := NewFieldBuilder(name)
	switch attrType.FriendlyName() {
	case "bool":
		return fb.AttributeField(
			generator.AttributeField{Type: generator.AttributeTypeBool}).Build()
	case "number":
		return fb.AttributeField(
			generator.AttributeField{Type: generator.AttributeTypeInt}).Build()
	case "string":
		return fb.AttributeField(
			generator.AttributeField{Type: generator.AttributeTypeString}).Build()
	case "map of bool":
		return fb.AttributeField(
			generator.AttributeField{
				Type:         generator.AttributeTypeMapStringKey,
				MapValueType: generator.AttributeTypeBool,
			}).Build()
	case "map of number":
		return fb.AttributeField(
			generator.AttributeField{
				Type:         generator.AttributeTypeMapStringKey,
				MapValueType: generator.AttributeTypeInt,
			}).Build()
	case "map of string":
		return fb.AttributeField(
			generator.AttributeField{
				Type:         generator.AttributeTypeMapStringKey,
				MapValueType: generator.AttributeTypeString,
			}).Build()
	case "list of number":
		return fb.IsSlice(true).AttributeField(
			generator.AttributeField{Type: generator.AttributeTypeInt}).Build()
	case "list of string":
		return fb.IsSlice(true).AttributeField(
			generator.AttributeField{Type: generator.AttributeTypeString}).Build()
	case "set of number":
		return fb.IsSlice(true).AttributeField(
			generator.AttributeField{Type: generator.AttributeTypeInt}).Build()
	case "set of string":
		return fb.IsSlice(true).AttributeField(
			generator.AttributeField{Type: generator.AttributeTypeString}).Build()
	case "set of map of string":
		return fb.IsSlice(true).AttributeField(
			generator.AttributeField{
				Type:         generator.AttributeTypeMapStringKey,
				MapValueType: generator.AttributeTypeString,
			}).Build()

	// TODO: the set/list of objects types can probably be []map[string]string
	// but we need to spot check and confirm this.
	case "list of object": // TODO: probably can be []map[string]string
		//f.AttributeField.Type = generator.AttributeTypeUnsupported
		if !attrType.IsListType() {
			return fb.Unsupported()
		}
		if !attrType.ElementType().IsObjectType() {
			return fb.Unsupported()
		}
		return fb.IsSlice(true).ObjectField(strcase.ToCamel(name), attrType).Build()
	case "set of object":
		// need better error handling here to help generate error messages
		// which would describe why the field is unsupported
		if !attrType.IsSetType() {
			return fb.Unsupported()
		}
		if !attrType.ElementType().IsObjectType() {
			return fb.Unsupported()
		}
		return fb.IsSlice(true).ObjectField(strcase.ToCamel(name), attrType).Build()
	default:
		// maybe this panic, either here or further up the stack
		return fb.Unsupported()
	}
}

func SpecOrStatus(attr *configschema.Attribute) SpecOrStatusField {
	// if attr.Computed is true, it can either be an attribute (status) or an argument (spec)
	// but arguments will always either be required or optional
	if attr.Required || attr.Optional {
		return ForProviderField
	}
	return AtProviderField
}

// SpecStatusAttributeFields iterates through the terraform configschema.Attribute map
// found under Block.Attributes, translating each attribute to a generator.Field and
// grouping them as spec or status based on their optional/required/computed properties.
func SpecOrStatusAttributeFields(attributes map[string]*configschema.Attribute) ([]generator.Field, []generator.Field) {
	forProvider := make([]generator.Field, 0)
	atProvider := make([]generator.Field, 0)
	for name, attr := range attributes {
		f := TypeToField(name, attr.Type)
		switch SpecOrStatus(attr) {
		case ForProviderField:
			forProvider = append(forProvider, f)
		case AtProviderField:
			atProvider = append(atProvider, f)
		}
	}
	return forProvider, atProvider
}

func NestedBlockFields(blocks map[string]*configschema.NestedBlock, packagePath string) []generator.Field {
	fields := make([]generator.Field, 0)
	for name, block := range blocks {
		f := generator.Field{
			Name:   strcase.ToCamel(name),
			Fields: make([]generator.Field, 0),
			Type:   generator.FieldTypeStruct,
			StructField: generator.StructField{
				PackagePath: packagePath,
				// TODO: the output would look nicer if we pluralized names when IsBlockList is true
				TypeName: strcase.ToCamel(name),
			},
			Tag: &generator.StructTag{
				&generator.StructTagJson{
					Name: name,
				},
			},
			Required: IsBlockRequired(block),
			IsSlice:  IsBlockSlice(block),
		}
		for n, attr := range block.Attributes {
			f.Fields = append(f.Fields, TypeToField(n, attr.Type))
		}
		f.Fields = append(f.Fields, NestedBlockFields(block.BlockTypes, packagePath)...)
		fields = append(fields, f)
	}
	return fields
}

func SchemaToManagedResource(name, packagePath string, s providers.Schema) *generator.ManagedResource {
	namer := generator.NewDefaultNamer(strcase.ToCamel(name))
	mr := generator.NewManagedResource(namer.TypeName(), packagePath).WithNamer(namer)
	spec, status := SpecOrStatusAttributeFields(s.Block.Attributes)
	mr.Parameters = generator.Field{
		Type: generator.FieldTypeStruct,
		StructField: generator.StructField{
			PackagePath: packagePath,
			TypeName:    namer.ForProviderTypeName(),
		},
		Fields: spec,
	}
	mr.Observation = generator.Field{
		Type: generator.FieldTypeStruct,
		StructField: generator.StructField{
			PackagePath: packagePath,
			TypeName:    namer.AtProviderTypeName(),
		},
		Fields: status,
	}
	nb := NestedBlockFields(s.Block.BlockTypes, packagePath)
	if len(nb) > 0 {
		mr.Parameters.Fields = append(mr.Parameters.Fields, nb...)
	}
	return mr
}

func BlockToField(name, typeName string, tfBlock *configschema.Block, enclosingField *generator.Field) *generator.Field {
	f := &generator.Field{
		Name: name,
		Type: generator.FieldTypeStruct,
		StructField: generator.StructField{
			PackagePath: enclosingField.StructField.PackagePath,
			TypeName:    typeName,
		},
	}
	return f
}

func IsBlockRequired(nb *configschema.NestedBlock) bool {
	if nb.MinItems > 0 {
		return true
	}
	return false
}

func IsBlockSlice(nb *configschema.NestedBlock) bool {
	if nb.MaxItems != 1 {
		return true
	}
	return false
}
