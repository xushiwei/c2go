package cl

import (
	goast "go/ast"
	"go/token"
	"go/types"
	"log"
	"strconv"

	"github.com/goplus/c2go/clang/ast"
	"github.com/goplus/gox"
)

// -----------------------------------------------------------------------------

const (
	flagLHS = 1 << iota
	flagIgnoreResult
)

const (
	unknownExprPrompt = "compileExpr: unknown kind ="
)

func compileExprEx(ctx *blockCtx, expr *ast.Node, prompt string, flags int) {
	switch expr.Kind {
	case ast.BinaryOperator:
		compileBinaryExpr(ctx, expr, flags)
	case ast.UnaryOperator:
		compileUnaryOperator(ctx, expr, flags)
	case ast.DeclRefExpr:
		compileDeclRefExpr(ctx, expr, (flags&flagLHS) != 0)
	case ast.MemberExpr:
		compileMemberExpr(ctx, expr, (flags&flagLHS) != 0)
	case ast.CallExpr:
		compileCallExpr(ctx, expr)
	case ast.CompoundAssignOperator:
		compileCompoundAssignOperator(ctx, expr, flags)
	case ast.ImplicitCastExpr:
		compileImplicitCastExpr(ctx, expr)
	case ast.IntegerLiteral:
		compileLiteral(ctx, token.INT, expr)
	case ast.StringLiteral:
		compileStringLiteral(ctx, expr)
	case ast.CharacterLiteral:
		compileCharacterLiteral(ctx, expr)
	case ast.FloatingLiteral:
		compileLiteral(ctx, token.FLOAT, expr)
	case ast.ParenExpr, ast.ConstantExpr:
		compileExprEx(ctx, expr.Inner[0], prompt, flags)
	case ast.CStyleCastExpr:
		compileTypeCast(ctx, expr, goNode(expr))
	case ast.ArraySubscriptExpr:
		compileArraySubscriptExpr(ctx, expr, (flags&flagLHS) != 0)
	case ast.UnaryExprOrTypeTraitExpr:
		compileUnaryExprOrTypeTraitExpr(ctx, expr)
	case ast.ImplicitValueInitExpr:
		compileImplicitValueInitExpr(ctx, expr)
	case ast.ConditionalOperator:
		compileConditionalOperator(ctx, expr)
	case ast.ImaginaryLiteral:
		compileImaginaryLiteral(ctx, expr)
	case ast.VAArgExpr:
		compileVAArgExpr(ctx, expr)
	case ast.AtomicExpr:
		compileAtomicExpr(ctx, expr)
	case ast.OffsetOfExpr:
		compileOffsetOfExpr(ctx, expr)
	default:
		log.Panicln(prompt, expr.Kind)
	}
}

func compileExpr(ctx *blockCtx, expr *ast.Node) {
	compileExprEx(ctx, expr, unknownExprPrompt, 0)
}

func compileExprLHS(ctx *blockCtx, expr *ast.Node) {
	compileExprEx(ctx, expr, unknownExprPrompt, flagLHS)
}

func compileLiteral(ctx *blockCtx, kind token.Token, expr *ast.Node) {
	ctx.cb.Val(&goast.BasicLit{Kind: kind, Value: expr.Value.(string)}, goNode(expr))
}

func compileCharacterLiteral(ctx *blockCtx, expr *ast.Node) {
	ctx.cb.Val(rune(expr.Value.(float64)), goNode(expr))
}

func compileStringLiteral(ctx *blockCtx, expr *ast.Node) {
	s, err := strconv.Unquote(expr.Value.(string))
	if err != nil {
		log.Panicln("compileStringLiteral:", err)
	}
	stringLit(ctx.cb, s, nil)
}

func compileImaginaryLiteral(ctx *blockCtx, expr *ast.Node) {
	compileExpr(ctx, expr.Inner[0])
	v := ctx.cb.Get(-1)
	lit := v.Val.(*goast.BasicLit)
	lit.Kind = token.IMAG
	lit.Value += "i"
}

// -----------------------------------------------------------------------------

func compileOffsetOfExpr(ctx *blockCtx, v *ast.Node) {
	tyStruct, name := ctx.paramsOfOfsetof(v)
	if debugCompileDecl {
		log.Println("==> offset", tyStruct, name)
	}
	t := toType(ctx, &ast.Type{QualType: tyStruct}, 0)
	ctx.cb.Val(ctx.offsetof(t, name))
}

func compileSizeof(ctx *blockCtx, v *ast.Node) {
	var t types.Type
	if len(v.Inner) > 0 {
		compileExpr(ctx, v.Inner[0])
		t = ctx.cb.InternalStack().Pop().Type
	} else {
		qualType := ctx.paramOfSizeof(v)
		if debugCompileDecl {
			log.Println("==> sizeof", qualType)
		}
		t = toType(ctx, &ast.Type{QualType: qualType}, 0)
	}
	ctx.cb.Val(ctx.sizeof(t))
}

func compileUnaryExprOrTypeTraitExpr(ctx *blockCtx, v *ast.Node) {
	switch v.Name {
	case "sizeof":
		compileSizeof(ctx, v)
	default:
		log.Panicln("unaryExprOrTypeTraitExpr unknown:", v.Name)
	}
}

func compileImplicitValueInitExpr(ctx *blockCtx, v *ast.Node) {
	t := toType(ctx, v.Type, 0)
	ctx.cb.ZeroLit(t)
}

func compileArraySubscriptExpr(ctx *blockCtx, v *ast.Node, lhs bool) {
	compileExpr(ctx, v.Inner[0])
	compileExpr(ctx, v.Inner[1])
	typeCastIndex(ctx, lhs)
}

// -----------------------------------------------------------------------------

func getBuiltinFn(v *ast.Node) (fn string, ok bool) {
	if v.Kind == ast.DeclRefExpr {
		if decl := v.ReferencedDecl; decl != nil {
			return decl.Name, true
		}
	}
	return
}

func compileImplicitCastExpr(ctx *blockCtx, v *ast.Node) {
	switch v.CastKind {
	case ast.LValueToRValue, ast.NoOp:
		compileExpr(ctx, v.Inner[0])
	case ast.BuiltinFnToFnPtr:
		if fn, ok := getBuiltinFn(v.Inner[0]); ok && ctx.pkg.Types.Scope().Lookup(fn) != nil {
			ctx.extfns[fn] = none{}
		}
		fallthrough
	case ast.FunctionToPointerDecay:
		compileExpr(ctx, v.Inner[0])
	case ast.ArrayToPointerDecay:
		compileExpr(ctx, v.Inner[0])
		if cb := ctx.cb; !isEllipsis(ctx, cb) {
			arrayToElemPtr(cb)
		}
	case ast.IntegralCast, ast.FloatingCast, ast.BitCast, ast.IntegralToFloating,
		ast.FloatingToIntegral, ast.FloatingComplexCast, ast.FloatingRealToComplex:
		compileTypeCast(ctx, v, nil)
	case ast.NullToPointer:
		ctx.cb.Val(nil)
	default:
		log.Panicln("compileImplicitCastExpr: unknown castKind =", v.CastKind)
	}
}

func compileTypeCast(ctx *blockCtx, v *ast.Node, src goast.Node) {
	switch v.CastKind {
	case ast.ToVoid: // _ = expr
		cb, _ := closureStartT(ctx, types.Typ[types.Int])
		cb.VarRef(nil)
		compileExpr(ctx, v.Inner[0])
		cb.Assign(1).Val(0).Return(1).End().Call(0)
		return
	}
	t := toType(ctx, v.Type, 0)
	ctx.cb.Typ(t, src)
	if v.CastKind == ast.NullToPointer {
		ctx.cb.Val(nil).Call(1)
		return
	}
	compileExpr(ctx, v.Inner[0])
	typeCastCall(ctx, t)
}

// -----------------------------------------------------------------------------

func compileDeclRefExpr(ctx *blockCtx, v *ast.Node, lhs bool) {
	name := v.ReferencedDecl.Name
	avoidKeyword(&name)
	obj := ctx.lookupParent(name)
	if obj == nil {
		log.Panicln("compileDeclRefExpr: not found -", name)
	}
	if lhs {
		ctx.cb.VarRef(obj)
	} else {
		ctx.cb.Val(obj)
	}
}

// -----------------------------------------------------------------------------

func compileCallExpr(ctx *blockCtx, v *ast.Node) {
	if n := len(v.Inner); n > 0 {
		if fn := v.Inner[0]; isBuiltinFn(fn) {
			item := fn.Inner[0]
			switch name := item.ReferencedDecl.Name; name {
			case "__builtin_va_start", "__builtin_va_end":
				return
			}
		}
		cb := ctx.cb
		for i := 0; i < n; i++ {
			compileExpr(ctx, v.Inner[i])
		}
		var flags gox.InstrFlags
		var ellipsis = n > 2 && isEllipsis(ctx, cb)
		if ellipsis {
			_, o := cb.Scope().LookupParent(valistName, token.NoPos)
			cb.InternalStack().Pop()
			cb.Val(o)
			flags = gox.InstrFlagEllipsis
		}
		cb.CallWith(n-1, flags, goNode(v))
	}
}

func isEllipsis(ctx *blockCtx, cb *gox.CodeBuilder) bool {
	return ctx.isValistType(cb.Get(-1).Type)
}

func isBuiltinFn(fn *ast.Node) bool {
	return fn.CastKind == ast.BuiltinFnToFnPtr
}

// -----------------------------------------------------------------------------

func compileMemberExpr(ctx *blockCtx, v *ast.Node, lhs bool) {
	avoidKeyword(&v.Name)
	name := v.Name
	compileExpr(ctx, v.Inner[0])
	if name == "" { // anonymous
		return
	}
	src := goNode(v)
	if lhs {
		ctx.cb.MemberRef(name, src)
	} else {
		ctx.cb.MemberVal(name, src)
	}
}

// -----------------------------------------------------------------------------

func compileCommaExpr(ctx *blockCtx, v *ast.Node, flags int) {
	cb, _ := closureStart(ctx, "")
	compileExprEx(ctx, v.Inner[0], unknownExprPrompt, flagIgnoreResult)
	cb.EndStmt()
	compileExpr(ctx, v.Inner[1])
	cb.Return(1).End().Call(0)
}

func compileBinaryExpr(ctx *blockCtx, v *ast.Node, flags int) {
	if op, ok := binaryOps[v.OpCode]; ok {
		isBoolOp := (op == token.LOR || op == token.LAND)
		compileExpr(ctx, v.Inner[0])
		if isBoolOp {
			castToBoolExpr(ctx.cb)
		}
		compileExpr(ctx, v.Inner[1])
		if isBoolOp {
			castToBoolExpr(ctx.cb)
			ctx.cb.BinaryOp(op, goNode(v))
		} else {
			binaryOp(ctx, op, v)
		}
		return
	}
	switch v.OpCode {
	case "=":
	case ",":
		compileCommaExpr(ctx, v, flags)
		return
	default:
		log.Panicln("compileBinaryExpr unknown operator:", v.OpCode)
	}
	if (flags & flagIgnoreResult) != 0 {
		compileSimpleAssignExpr(ctx, v)
		return
	}
	compileAssignExpr(ctx, v)
}

func isCmpOperator(op token.Token) bool {
	return op >= token.EQL && op <= token.GEQ
}

var (
	binaryOps = map[ast.OpCode]token.Token{
		"+": token.ADD,
		"-": token.SUB,
		"*": token.MUL,
		"/": token.QUO,
		"%": token.REM,

		"&":  token.AND,
		"|":  token.OR,
		"^":  token.XOR,
		"<<": token.SHL,
		">>": token.SHR,

		"==": token.EQL,
		"<":  token.LSS,
		">":  token.GTR,
		"!=": token.NEQ,
		"<=": token.LEQ,
		">=": token.GEQ,

		"||": token.LOR,
		"&&": token.LAND,
	}
)

// -----------------------------------------------------------------------------

func compileCompoundAssignOperator(ctx *blockCtx, v *ast.Node, flags int) {
	if op, ok := assignOps[v.OpCode]; ok {
		if (flags & flagIgnoreResult) != 0 {
			compileSimpleAssignOpExpr(ctx, op, v)
		} else {
			compileAssignOpExpr(ctx, op, v)
		}
		return
	}
	log.Panicln("compileCompoundAssignOperator unknown operator:", v.OpCode)
}

var (
	assignOps = map[ast.OpCode]token.Token{
		"+=": token.ADD_ASSIGN,
		"-=": token.SUB_ASSIGN,
		"*=": token.MUL_ASSIGN,
		"/=": token.QUO_ASSIGN,
		"%=": token.REM_ASSIGN,

		"&=":  token.AND_ASSIGN,
		"|=":  token.OR_ASSIGN,
		"^=":  token.XOR_ASSIGN,
		"<<=": token.SHL_ASSIGN,
		">>=": token.SHR_ASSIGN,
	}
)

// -----------------------------------------------------------------------------

const (
	addrVarName = "_cgo_addr"
)

func compileSimpleAssignExpr(ctx *blockCtx, v *ast.Node) {
	compileExprLHS(ctx, v.Inner[0])
	compileExpr(ctx, v.Inner[1])
	assign(ctx, goNode(v.Inner[1]))
}

func compileAssignExpr(ctx *blockCtx, v *ast.Node) {
	cb, _ := closureStartInitAddr(ctx, v)

	addr := cb.Scope().Lookup(addrVarName)
	cb.Val(addr).ElemRef()
	compileExpr(ctx, v.Inner[1])
	assign(ctx, goNode(v.Inner[1]))

	cb.Val(addr).Elem().Return(1).End().Call(0)
}

func compileSimpleAssignOpExpr(ctx *blockCtx, op token.Token, v *ast.Node) {
	compileExprLHS(ctx, v.Inner[0])
	compileExpr(ctx, v.Inner[1])
	assignOp(ctx, op, goNode(v.Inner[1]))
}

func compileAssignOpExpr(ctx *blockCtx, op token.Token, v *ast.Node) {
	cb, _ := closureStartInitAddr(ctx, v)

	addr := cb.Scope().Lookup(addrVarName)
	cb.Val(addr).ElemRef()
	compileExpr(ctx, v.Inner[1])
	assignOp(ctx, op, goNode(v.Inner[1]))

	cb.Val(addr).Elem().Return(1).End().Call(0)
}

func compileSimpleIncDec(ctx *blockCtx, op token.Token, v *ast.Node) {
	cb := ctx.cb
	stk := cb.InternalStack()
	compileExprLHS(ctx, v.Inner[0])
	typ, _ := gox.DerefType(stk.Get(-1).Type)
	if t, ok := typ.(*types.Pointer); ok { // *type
		cb.UnaryOp(token.AND)
		castPtrType(cb, tyUintptrPtr, stk.Pop())
		cb.ElemRef()
		if elemSize := ctx.sizeof(t.Elem()); elemSize != 1 {
			cb.Val(elemSize).AssignOp(op + (token.ADD_ASSIGN - token.INC))
			return
		}
	}
	cb.IncDec(op)
}

func compileIncDec(ctx *blockCtx, op token.Token, v *ast.Node) {
	cb, ret := closureStartInitAddr(ctx, v)
	n := 0
	addr := cb.Scope().Lookup(addrVarName)
	if v.IsPostfix {
		cb.VarRef(ret).Val(addr).Elem().Assign(1)
	}
	elemSize := valOfAddr(cb, addr, ctx)
	cb.ElemRef()
	if elemSize == 1 {
		cb.IncDec(op)
	} else {
		cb.Val(elemSize).AssignOp(op + (token.ADD_ASSIGN - token.INC))
	}
	if !v.IsPostfix {
		cb.Val(addr).Elem()
		n = 1
	}
	cb.Return(n).End().Call(0)
}

func closureStartInitAddr(ctx *blockCtx, v *ast.Node) (*gox.CodeBuilder, *types.Var) {
	cb, ret := closureStart(ctx, "_cgo_ret")
	cb.DefineVarStart(token.NoPos, addrVarName)
	compileExprLHS(ctx, v.Inner[0])
	cb.UnaryOp(token.AND).EndInit(1)
	return cb, ret
}

func closureStart(ctx *blockCtx, retName string) (*gox.CodeBuilder, *types.Var) {
	pkg := ctx.pkg
	ret := pkg.NewAutoParam(retName)
	return ctx.cb.NewClosure(nil, types.NewTuple(ret), false).BodyStart(pkg), ret
}

func closureStartT(ctx *blockCtx, t types.Type) (*gox.CodeBuilder, *types.Var) {
	pkg := ctx.pkg
	ret := pkg.NewParam(token.NoPos, "", t)
	return ctx.cb.NewClosure(nil, types.NewTuple(ret), false).BodyStart(pkg), ret
}

// -----------------------------------------------------------------------------

func compileConditionalOperator(ctx *blockCtx, v *ast.Node) {
	t := toType(ctx, v.Type, 0)
	cb, _ := closureStartT(ctx, t)
	cb.If()
	compileExpr(ctx, v.Inner[0])
	castToBoolExpr(cb)
	cb.Then()
	compileExpr(ctx, v.Inner[1])
	cb.Return(1).Else()
	compileExpr(ctx, v.Inner[2])
	cb.Return(1).End().
		End().Call(0) // end func
}

// -----------------------------------------------------------------------------

func compileStarExpr(ctx *blockCtx, v *ast.Node, lhs bool) {
	cb := ctx.cb
	compileExpr(ctx, v.Inner[0])
	src := goNode(v)
	if lhs {
		cb.ElemRef(src)
	} else {
		if !isFunc(cb.Get(-1).Type) { // *fn => fn
			cb.Elem(src)
		}
	}
}

func compileUnaryOperator(ctx *blockCtx, v *ast.Node, flags int) {
	lhs := (flags & flagLHS) != 0
	if v.OpCode == "*" {
		compileStarExpr(ctx, v, lhs)
		return
	}
	if lhs {
		log.Panicln("compileUnaryOperator: not a lhs expression -", v.OpCode)
	}
	if op, ok := unaryOps[v.OpCode]; ok {
		compileExpr(ctx, v.Inner[0])
		unaryOp(ctx, op, v)
		return
	}

	var tok token.Token
	switch v.OpCode {
	case "++":
		tok = token.INC
	case "--":
		tok = token.DEC
	case "__extension__":
		compileExpr(ctx, v.Inner[0])
		return
	case "+":
		compileExpr(ctx, v.Inner[0])
		return
	default:
		log.Panicln("compileUnaryOperator: unknown operator -", v.OpCode)
	}
	if (flags & flagIgnoreResult) != 0 {
		compileSimpleIncDec(ctx, tok, v)
		return
	}
	compileIncDec(ctx, tok, v)
}

var (
	unaryOps = map[ast.OpCode]token.Token{
		"-": token.SUB,
		"&": token.AND,
		"~": token.XOR,
		"!": token.NOT,
	}
)

// -----------------------------------------------------------------------------

func compileAtomicExpr(ctx *blockCtx, v *ast.Node) {
	op := ctx.getInstr(v)
	cb := ctx.cb.Val(ctx.pkg.Ref(op))
	for _, expr := range v.Inner {
		compileExpr(ctx, expr)
	}
	cb.Call(len(v.Inner))
	if fn, ok := getCaller(cb); ok {
		ctx.extfns[fn] = none{}
	}
}

func getCaller(cb *gox.CodeBuilder) (string, bool) {
	v := cb.Get(-1)
	if e, ok := v.Val.(*goast.CallExpr); ok {
		if fn, ok := e.Fun.(*goast.Ident); ok {
			return fn.Name, true
		}
	}
	return "", false
}

// -----------------------------------------------------------------------------
