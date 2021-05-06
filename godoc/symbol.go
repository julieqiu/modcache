package godoc

import (
	"fmt"
	"go/ast"
	"go/doc"
)

type Symbol struct {
	SymbolMeta
	Children []*SymbolMeta
}

type SymbolMeta struct {
	Name       string
	ParentName string
}

func GetSymbols(p *doc.Package) (_ []string, err error) {
	typs, err := types(p)
	if err != nil {
		return nil, err
	}
	vars, err := variables(p.Vars)
	if err != nil {
		return nil, err
	}
	syms := append(append(append(
		constants(p.Consts), vars...), functions(p)...), typs...)
	var out []string
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out, nil
}

func constants(consts []*doc.Value) []*Symbol {
	var syms []*Symbol
	for _, c := range consts {
		for _, n := range c.Names {
			if n == "_" {
				continue
			}
			syms = append(syms, &Symbol{
				SymbolMeta: SymbolMeta{
					Name: n,
				},
			})
		}
	}
	return syms
}

func variables(vars []*doc.Value) (_ []*Symbol, err error) {
	var syms []*Symbol
	for _, v := range vars {
		specs := v.Decl.Specs
		for _, spec := range specs {
			valueSpec := spec.(*ast.ValueSpec) // must succeed; we can't mix types in one GenDecl.
			for _, ident := range valueSpec.Names {
				if ident.Name == "_" {
					continue
				}
				vs := *valueSpec
				if len(valueSpec.Names) != 0 {
					vs.Names = []*ast.Ident{ident}
				}
				syms = append(syms,
					&Symbol{
						SymbolMeta: SymbolMeta{
							Name: ident.Name,
						},
					})
			}

		}
	}
	return syms, nil
}

func functions(p *doc.Package) []*Symbol {
	var syms []*Symbol
	for _, f := range p.Funcs {
		syms = append(syms, &Symbol{
			SymbolMeta: SymbolMeta{
				Name: f.Name,
			},
		})
	}
	return syms
}

func types(p *doc.Package) ([]*Symbol, error) {
	var syms []*Symbol
	for _, typ := range p.Types {
		specs := typ.Decl.Specs
		if len(specs) != 1 {
			return nil, fmt.Errorf("unexpected number of t.Decl.Specs: %d (wanted len = 1)", len(typ.Decl.Specs))
		}
		spec, ok := specs[0].(*ast.TypeSpec)
		if !ok {
			return nil, fmt.Errorf("unexpected type for Spec node: %q", typ.Name)
		}
		mthds, err := methodsForType(typ, spec)
		if err != nil {
			return nil, err
		}
		t := &Symbol{
			SymbolMeta: SymbolMeta{
				Name: typ.Name,
			},
		}
		fields := fieldsForType(typ.Name, spec)
		if err != nil {
			return nil, err
		}
		syms = append(syms, t)
		vars, err := variablesForType(typ)
		if err != nil {
			return nil, err
		}
		t.Children = append(append(append(append(append(
			t.Children,
			constantsForType(typ)...),
			vars...),
			functionsForType(typ)...),
			fields...),
			mthds...)
	}
	return syms, nil
}

func constantsForType(t *doc.Type) []*SymbolMeta {
	consts := constants(t.Consts)
	var typConsts []*SymbolMeta
	for _, c := range consts {
		c2 := c.SymbolMeta
		c2.ParentName = t.Name
		typConsts = append(typConsts, &c2)
	}
	return typConsts
}

func variablesForType(t *doc.Type) (_ []*SymbolMeta, err error) {
	vars, err := variables(t.Vars)
	if err != nil {
		return nil, err
	}
	var typVars []*SymbolMeta
	for _, v := range vars {
		v2 := v.SymbolMeta
		v2.ParentName = t.Name
		typVars = append(typVars, &v2)
	}
	return typVars, nil
}

func functionsForType(t *doc.Type) []*SymbolMeta {
	var syms []*SymbolMeta
	for _, f := range t.Funcs {
		syms = append(syms, &SymbolMeta{
			Name:       f.Name,
			ParentName: t.Name,
		})
	}
	return syms
}

func fieldsForType(typName string, spec *ast.TypeSpec) []*SymbolMeta {
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	var syms []*SymbolMeta
	for _, f := range st.Fields.List {
		// It's not possible for there to be more than one name.
		// FieldList is also used by go/ast for st.Methods, which is the
		// only reason this type is a list.
		for _, n := range f.Names {
			name := typName + "." + n.Name
			syms = append(syms, &SymbolMeta{
				Name:       name,
				ParentName: typName,
			})
		}
	}
	return syms
}

func methodsForType(t *doc.Type, spec *ast.TypeSpec) ([]*SymbolMeta, error) {
	var syms []*SymbolMeta
	for _, m := range t.Methods {
		syms = append(syms, &SymbolMeta{
			Name:       t.Name + "." + m.Name,
			ParentName: t.Name,
		})
	}
	if st, ok := spec.Type.(*ast.InterfaceType); ok {
		for _, m := range st.Methods.List {
			// It's not possible for there to be more than one name.
			// FieldList is also used by go/ast for st.Methods, which is the
			// only reason this type is a list.
			if len(m.Names) > 1 {
				return nil, fmt.Errorf("len(m.Names) = %d; expected 0 or 1", len(m.Names))
			}
			for _, n := range m.Names {
				name := t.Name + "." + n.Name
				syms = append(syms, &SymbolMeta{
					Name:       name,
					ParentName: t.Name,
				})
			}
		}
	}
	return syms, nil
}
