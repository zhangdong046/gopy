// Copyright 2017 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bind

import (
	"go/token"
)

const (
	// FIXME(corona10): ffibuilder.cdef should be written this way.
	// ffi.cdef("""
	//       //header exported from 'go tool cgo'
	//      #include "%[3]s.h"
	//       """)
	// discuss: https://github.com/go-python/gopy/pull/93#discussion_r119652220
	cffiPreamble = `"""%[1]s"""
import os
import cffi as _cffi_backend

ffi = _cffi_backend.FFI()
ffi.cdef("""
typedef signed char GoInt8;
typedef unsigned char GoUint8;
typedef short GoInt16;
typedef unsigned short GoUint16;
typedef int GoInt32;
typedef unsigned int GoUint32;
typedef long long GoInt64;
typedef size_t GoUintptr;
typedef unsigned long long GoUint64;
typedef GoInt64 GoInt;
typedef GoUint64 GoUint;
typedef float GoFloat32;
typedef double GoFloat64;
typedef struct { const char *p; GoInt n; } GoString;
typedef void *GoMap;
typedef void *GoChan;
typedef struct { void *t; void *v; } GoInterface;
typedef struct { void *data; GoInt len; GoInt cap; } GoSlice;

extern GoString _cgopy_GoString(char* p0);
extern char* _cgopy_CString(GoString p0);
extern void _cgopy_FreeCString(char* p0);
extern GoUint8 _cgopy_ErrorIsNil(GoInterface p0);
extern char* _cgopy_ErrorString(GoInterface p0);
extern void cgopy_incref(void* p0);
extern void cgopy_decref(void* p0);

extern void cgo_pkg_%[2]s_init();
`
	cffiHelperPreamble = `""")

# python <--> cffi helper.
class _cffi_helper(object):

    here = os.path.dirname(os.path.abspath(__file__))
    lib = ffi.dlopen(os.path.join(here, "_%[1]s.so"))

    @staticmethod
    def cffi_cgopy_cnv_py2c_string(o):
        s = ffi.new("char[]", o)
        return _cffi_helper.lib._cgopy_GoString(s)

    @staticmethod
    def cffi_cgopy_cnv_py2c_int(o):
        return ffi.cast('int', o)

    @staticmethod
    def cffi_cgopy_cnv_py2c_float32(o):
        return ffi.cast('float', o)

    @staticmethod
    def cffi_cgopy_cnv_py2c_float64(o):
        return ffi.cast('double', o)

    @staticmethod
    def cffi_cgopy_cnv_py2c_uint(o):
        return ffi.cast('uint', o)

    @staticmethod
    def cffi_cgopy_cnv_c2py_string(c):
        s = _cffi_helper.lib._cgopy_CString(c)
        pystr = ffi.string(s)
        _cffi_helper.lib._cgopy_FreeCString(s)
        return pystr

    @staticmethod
    def cffi_cgopy_cnv_c2py_int(c):
        return int(c)

    @staticmethod
    def cffi_cgopy_cnv_c2py_float32(c):
        return float(c)

    @staticmethod
    def cffi_cgopy_cnv_c2py_float64(c):
        return float(c)

    @staticmethod
    def cffi_cgopy_cnv_c2py_uint(c):
        return int(c)

# make sure Cgo is loaded and initialized
_cffi_helper.lib.cgo_pkg_%[1]s_init()
`
)

type cffiGen struct {
	wrapper *printer

	fset *token.FileSet
	pkg  *Package
	err  ErrorList

	lang int // c-python api version (2,3)
}

func (g *cffiGen) gen() error {
	// Write preamble for CFFI library wrapper.
	g.genCffiPreamble()
	g.genCffiCdef()
	g.genWrappedPy()
	return nil
}

func (g *cffiGen) genCffiPreamble() {
	n := g.pkg.pkg.Name()
	pkgDoc := g.pkg.doc.Doc
	g.wrapper.Printf(cffiPreamble, pkgDoc, n)
}

func (g *cffiGen) genCffiCdef() {

	// first, process slices, arrays
	{
		names := g.pkg.syms.names()
		for _, n := range names {
			sym := g.pkg.syms.sym(n)
			if !sym.isType() {
				continue
			}
			g.genCdefType(sym)
		}
	}

	for _, f := range g.pkg.funcs {
		g.genCdefFunc(f)
	}

	for _, c := range g.pkg.consts {
		g.genCdefConst(c)
	}

	for _, v := range g.pkg.vars {
		g.genCdefVar(v)
	}
}

func (g *cffiGen) genWrappedPy() {
	n := g.pkg.pkg.Name()
	g.wrapper.Printf(cffiHelperPreamble, n)

	for _, f := range g.pkg.funcs {
		g.genFunc(f)
	}

	for _, c := range g.pkg.consts {
		g.genConst(c)
	}

	for _, v := range g.pkg.vars {
		g.genVar(v)
	}
}

func (g *cffiGen) genConst(c Const) {
	g.genGetFunc(c.f)
}

func (g *cffiGen) genVar(v Var) {
	id := g.pkg.Name() + "_" + v.Name()
	doc := v.doc
	{
		res := []*Var{newVar(g.pkg, v.GoType(), "ret", v.Name(), doc)}
		sig := newSignature(g.pkg, nil, nil, res)
		fget := Func{
			pkg:  g.pkg,
			sig:  sig,
			typ:  nil,
			name: v.Name(),
			id:   id + "_get",
			doc:  "returns " + g.pkg.Name() + "." + v.Name(),
			ret:  v.GoType(),
			err:  false,
		}
		g.genGetFunc(fget)
	}
	{
		params := []*Var{newVar(g.pkg, v.GoType(), "arg", v.Name(), doc)}
		sig := newSignature(g.pkg, nil, params, nil)
		fset := Func{
			pkg:  g.pkg,
			sig:  sig,
			typ:  nil,
			name: v.Name(),
			id:   id + "_set",
			doc:  "sets " + g.pkg.Name() + "." + v.Name(),
			ret:  nil,
			err:  false,
		}
		g.genSetFunc(fset)
	}
}

func (g *cffiGen) genCdefConst(c Const) {
	g.genCdefFunc(c.f)
}

func (g *cffiGen) genCdefVar(v Var) {
	id := g.pkg.Name() + "_" + v.Name()
	doc := v.doc
	{
		res := []*Var{newVar(g.pkg, v.GoType(), "ret", v.Name(), doc)}
		sig := newSignature(g.pkg, nil, nil, res)
		fget := Func{
			pkg:  g.pkg,
			sig:  sig,
			typ:  nil,
			name: v.Name(),
			id:   id + "_get",
			doc:  "returns " + g.pkg.Name() + "." + v.Name(),
			ret:  v.GoType(),
			err:  false,
		}
		g.genCdefFunc(fget)
	}
	{
		params := []*Var{newVar(g.pkg, v.GoType(), "arg", v.Name(), doc)}
		sig := newSignature(g.pkg, nil, params, nil)
		fset := Func{
			pkg:  g.pkg,
			sig:  sig,
			typ:  nil,
			name: v.Name(),
			id:   id + "_set",
			doc:  "sets " + g.pkg.Name() + "." + v.Name(),
			ret:  nil,
			err:  false,
		}
		g.genCdefFunc(fset)
	}
}
