package evaluator

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/javanhut/Carrion/src/ast"
	"github.com/javanhut/Carrion/src/lexer"
	"github.com/javanhut/Carrion/src/object"
	"github.com/javanhut/Carrion/src/parser"
	"github.com/javanhut/Carrion/src/token"
)

// EvalContext tracks call stack and other context information during evaluation
type EvalContext struct {
	callStack []CallFrame
	fileName  string
}

// CallFrame represents a function call in the call stack
type CallFrame struct {
	funcName string
	position token.Position
}

// NewEvalContext creates a new evaluation context
func NewEvalContext(fileName string) *EvalContext {
	return &EvalContext{
		callStack: []CallFrame{},
		fileName:  fileName,
	}
}

// PushCallFrame adds a new frame to the call stack
func (ctx *EvalContext) PushCallFrame(funcName string, position token.Position) {
	position.File = ctx.fileName // Ensure the filename is set
	ctx.callStack = append(ctx.callStack, CallFrame{
		funcName: funcName,
		position: position,
	})
}

// PopCallFrame removes the most recent frame from the call stack
func (ctx *EvalContext) PopCallFrame() {
	if len(ctx.callStack) > 0 {
		ctx.callStack = ctx.callStack[:len(ctx.callStack)-1]
	}
}

// GetCallStack returns the current call stack
func (ctx *EvalContext) GetCallStack() []object.StackTraceEntry {
	entries := make([]object.StackTraceEntry, len(ctx.callStack))
	for i, frame := range ctx.callStack {
		entries[i] = object.StackTraceEntry{
			Function: frame.funcName,
			Position: frame.position,
		}
	}
	return entries
}

// CurrentPosition returns the position of the current execution point
func (ctx *EvalContext) CurrentPosition() token.Position {
	if len(ctx.callStack) > 0 {
		return ctx.callStack[len(ctx.callStack)-1].position
	}
	return token.Position{File: ctx.fileName}
}

var (
	NONE          = &object.None{Value: "None"}
	TRUE          = &object.Boolean{Value: true}
	FALSE         = &object.Boolean{Value: false}
	importedFiles = map[string]bool{}
)

func Eval(node ast.Node, env *object.Environment) object.Object {
	switch node := node.(type) {

	case *ast.Program:
		return evalProgram(node, env)
	case *ast.ExpressionStatement:
		return Eval(node.Expression, env)
	case *ast.BlockStatement:
		return evalBlockStatement(node, env)
	case *ast.IfStatement:
		return evalIfExpression(node, env)

	case *ast.StopStatement:
		return object.STOP
	case *ast.SkipStatement:
		return object.SKIP
	case *ast.CheckStatement:
		cond := Eval(node.Condition, env)
		if isError(cond) {
			return cond
		}
		if !isTruthy(cond) {
			msg := "Assertion failed: " + node.Condition.String()
			if node.Message != nil {
				m := Eval(node.Message, env)
				if !isError(m) {
					msg = m.Inspect()
				}
			}

			return object.NewCustomError("Assertion Check Failed: ", msg)
		}
		return object.NONE

	case *ast.PrefixExpression:
		if node.Operator == "++" || node.Operator == "--" {
			return evalPrefixIncrementDecrement(node.Operator, node, env)
		}
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalPrefixExpression(node.Operator, node, env)

	case *ast.InfixExpression:
		if node.Operator == "+=" || node.Operator == "-=" ||
			node.Operator == "*=" || node.Operator == "/=" {
			return evalCompoundAssignment(node, env)
		}

		if node.Operator == "and" {
			left := Eval(node.Left, env)
			if isError(left) {
				return left
			}
			if !isTruthy(left) {
				return left
			}
			return Eval(node.Right, env)
		}

		if node.Operator == "or" {
			left := Eval(node.Left, env)
			if isError(left) {
				return left
			}
			if isTruthy(left) {
				return left
			}
			return Eval(node.Right, env)
		}

		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		result := evalInfixExpression(node.Operator, left, right)

		return result
	case *ast.PostfixExpression:
		return evalPostfixIncrementDecrement(node.Operator, node, env)

	case *ast.IntegerLiteral:
		return &object.Integer{Value: node.Value}
	case *ast.FloatLiteral:
		return &object.Float{Value: node.Value}
	case *ast.FStringLiteral:
		return evalFStringLiteral(node, env)
	case *ast.NoneLiteral:
		return object.NONE
	case *ast.ReturnStatement:
		val := Eval(node.ReturnValue, env)
		if isError(val) {
			return val
		}
		return &object.ReturnValue{Value: val}
	case *ast.Boolean:
		return nativeBoolToBooleanObject(node.Value)
	case *ast.AssignStatement:
		return evalAssignStatement(node, env)
	case *ast.WhileStatement:
		return evalWhileStatement(node, env)
	case *ast.ForStatement:
		return evalForStatement(node, env)
	case *ast.ImportStatement:
		return evalImportStatement(node, env)
	case *ast.MatchStatement:
		return evalMatchStatement(node, env)
	case *ast.RaiseStatement:
		return evalRaiseStatement(node, env)
	case *ast.ArcaneGrimoire:
		return evalArcaneGrimoire(node, env)
	case *ast.Identifier:
		return evalIdentifier(node, env)
	case *ast.ArrayLiteral:
		elements := evalExpressions(node.Elements, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &object.Array{Elements: elements}

	case *ast.StringLiteral:
		return &object.String{Value: node.Value}
	case *ast.TupleLiteral:
		return evalTupleLiteral(node, env)
	case *ast.HashLiteral:
		return evalHashLiteral(node, env)
	case *ast.FunctionDefinition:
		fnObj := &object.Function{
			Parameters: node.Parameters,
			Body:       node.Body,
			Env:        env,
		}
		env.Set(node.Name.Value, fnObj)
		return fnObj
	case *ast.DotExpression:
		return evalDotExpression(node, env)
	case *ast.IndexExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		
		// Check if the index is a RangeExpression for array slicing
		if rangeExp, ok := node.Index.(*ast.RangeExpression); ok {
			// Create Range object
			rangeObj := &object.Range{}
			
			// Evaluate start index if present
			if rangeExp.Start != nil {
				startIdx := Eval(rangeExp.Start, env)
				if isError(startIdx) {
					return startIdx
				}
				rangeObj.Start = startIdx
			} else {
				rangeObj.Start = NONE
			}
			
			// Evaluate end index if present
			if rangeExp.End != nil {
				endIdx := Eval(rangeExp.End, env)
				if isError(endIdx) {
					return endIdx
				}
				rangeObj.End = endIdx
			} else {
				rangeObj.End = NONE
			}
			
			// Evaluate array slicing with the range object
			return evalIndexExpression(left, rangeObj)
		} else {
			// Regular index evaluation
			index := Eval(node.Index, env)
			if isError(index) {
				return index
			}
			return evalIndexExpression(left, index)
		}
	case *ast.GrimoireDefinition:
		return evalGrimoireDefinition(node, env)
	case *ast.AttemptStatement:
		return evalAttemptStatement(node, env)
	case *ast.IgnoreStatement:
		return object.NONE
	case *ast.CallExpression:
		return evalCallExpression(Eval(node.Function, env), evalExpressions(node.Arguments, env), env)

	}
	return NONE
}

func evalFStringLiteral(fslit *ast.FStringLiteral, env *object.Environment) object.Object {
	var sb strings.Builder

	for _, part := range fslit.Parts {
		switch p := part.(type) {
		case *ast.FStringText:
			sb.WriteString(p.Value)
		case *ast.FStringExpr:
			val := Eval(p.Expr, env)
			if isError(val) {
				return val
			}
			sb.WriteString(val.Inspect())
		}
	}

	return &object.String{Value: sb.String()}
}

func evalArcaneGrimoire(node *ast.ArcaneGrimoire, env *object.Environment) object.Object {
	methods := make(map[string]*object.Function)

	for _, method := range node.Methods {
		methods[method.Name.Value] = &object.Function{
			Parameters: method.Parameters,
			Body:       method.Body,
			Env:        env,
		}
	}

	grimoire := &object.Grimoire{
		Name:     node.Name.Value,
		Methods:  methods,
		Env:      env,
		IsArcane: true,
	}

	env.Set(node.Name.Value, grimoire)
	return grimoire
}

func evalRaiseStatement(node *ast.RaiseStatement, env *object.Environment) object.Object {
	errObj := Eval(node.Error, env)
	if isError(errObj) {
		return errObj
	}

	// Get position information from the token
	position := node.Token.Position
	
	// Get function name for stack trace from environment
	functionName := env.GetFunctionName()
	if functionName == "" {
		// Default to main for top-level code
		functionName = "main"
	}

	if instance, ok := errObj.(*object.Instance); ok {
		message := ""
		if msg, ok := instance.Env.Get("message"); ok {
			if msgStr, ok := msg.(*object.String); ok {
				message = msgStr.Value
			}
		}
		
		customErr := &object.CustomError{
			Name:      instance.Grimoire.Name,
			Message:   message,
			ErrorType: instance.Grimoire,
			Instance:  instance,
			Position:  position,
			StackTrace: []object.StackTraceEntry{},
		}
		
		// Add current position to stack trace with function context
		customErr.AddStackEntry(position, functionName)
		
		return customErr
	}

	if str, ok := errObj.(*object.String); ok {
		customErr := object.NewCustomError("Error", str.Value, position)
		customErr.AddStackEntry(position, functionName)
		return customErr
	}

	err := newError("cannot raise non-error object: %s", errObj.Type())
	err.Position = position
	err.AddStackEntry(position, functionName)
	return err
}

func evalAttemptStatement(node *ast.AttemptStatement, env *object.Environment) object.Object {
	var result object.Object

	tryResult := Eval(node.TryBlock, env)

	if isError(tryResult) {
		if customErr, ok := tryResult.(*object.CustomError); ok {
			for _, ensnare := range node.EnsnareClauses {

				condition := Eval(ensnare.Condition, env)
				if isError(condition) {
					result = condition
					break
				}

				if grimoire, ok := condition.(*object.Grimoire); ok {
					if customErr.ErrorType == grimoire {
						result = Eval(ensnare.Consequence, env)
						break
					}
				} else if str, ok := condition.(*object.String); ok {
					if customErr.Name == str.Value {
						result = Eval(ensnare.Consequence, env)
						break
					}
				}
			}
		}

		if result == nil {
			result = tryResult
		}
	} else {
		result = tryResult
	}

	if node.ResolveBlock != nil {
		resolveResult := Eval(node.ResolveBlock, env)
		if isError(resolveResult) {
			return resolveResult
		}
	}

	return result
}

func evalMatchStatement(ms *ast.MatchStatement, env *object.Environment) object.Object {
	matchValue := Eval(ms.MatchValue, env)
	if isError(matchValue) {
		return matchValue
	}

	for _, caseClause := range ms.Cases {
		caseCondition := Eval(caseClause.Condition, env)
		if isError(caseCondition) {
			return caseCondition
		}

		if isEqual(matchValue, caseCondition) {
			return Eval(caseClause.Body, env)
		}
	}

	if ms.Default != nil {
		return Eval(ms.Default.Body, env)
	}

	return NONE
}

func isEqual(obj1, obj2 object.Object) bool {
	switch obj1 := obj1.(type) {
	case *object.Integer:
		if obj2, ok := obj2.(*object.Integer); ok {
			return obj1.Value == obj2.Value
		}
	case *object.String:
		if obj2, ok := obj2.(*object.String); ok {
			return obj1.Value == obj2.Value
		}

	default:
		return false
	}
	return false
}

func evalAssignStatement(node *ast.AssignStatement, env *object.Environment) object.Object {
	switch target := node.Name.(type) {

	case *ast.Identifier:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}

		env.Set(target.Value, val)
		return val

	case *ast.DotExpression:
		left := Eval(target.Left, env)
		if isError(left) {
			return left
		}
		instance, ok := left.(*object.Instance)
		if !ok {
			return newError("invalid assignment target: %s", left.Type())
		}
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		instance.Env.Set(target.Right.Value, val)
		return val

	case *ast.TupleLiteral:

		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}

		var values []object.Object
		switch v := val.(type) {
		case *object.Tuple:
			values = v.Elements
		case *object.Array:
			values = v.Elements
		default:
			return newError("cannot unpack non-iterable type: %s", val.Type())
		}

		if len(target.Elements) != len(values) {
			return newError("unpacking mismatch: expected %d values, got %d", len(target.Elements), len(values))
		}

		for i, expr := range target.Elements {
			ident, ok := expr.(*ast.Identifier)
			if !ok {
				return newError("invalid assignment target in tuple assignment")
			}
			env.Set(ident.Value, values[i])
		}
		return val

	default:
		return newError("invalid assignment target: %T", node.Name)
	}
}

func checkType(val object.Object, expectedType string) bool {
	switch expectedType {
	case "str":
		return val.Type() == object.STRING_OBJ
	case "int":
		return val.Type() == object.INTEGER_OBJ
	case "float":
		return val.Type() == object.FLOAT_OBJ
	case "bool":
		return val.Type() == object.BOOLEAN_OBJ

	default:

		return true
	}
}

func getGlobalEnv(env *object.Environment) *object.Environment {
	for env.GetOuter() != nil {
		env = env.GetOuter()
	}
	return env
}

func evalGrimoireDefinition(node *ast.GrimoireDefinition, env *object.Environment) object.Object {
	methods := map[string]*object.Function{}

	var parentGrimoire *object.Grimoire
	if node.Inherits != nil {
		parentObj, ok := env.Get(node.Inherits.Value)
		if !ok {
			return newError("parent grimoire '%s' not found", node.Inherits.Value)
		}
		parentGrimoire, ok = parentObj.(*object.Grimoire)
		if !ok {
			return newError("'%s' is not a grimoire", node.Inherits.Value)
		}

		for name, method := range parentGrimoire.Methods {
			methods[name] = method
		}
	}

	for _, method := range node.Methods {
		fn := &object.Function{
			Parameters: method.Parameters,
			Body:       method.Body,
			Env:        env,
		}
		if strings.HasPrefix(method.Name.Value, "__") {
			fn.IsPrivate = true
		} else if strings.HasPrefix(method.Name.Value, "_") {
			fn.IsProtected = true
		}

		if method.Token.Type == token.ARCANESPELL {
			fn.IsAbstract = true
		}
		methods[method.Name.Value] = fn
	}

	if parentGrimoire != nil {
		for name, method := range parentGrimoire.Methods {
			if method.IsAbstract {
				if _, ok := methods[name]; !ok {
					return newError(
						"grimoire '%s' must implement abstract method '%s'",
						node.Name.Value, name,
					)
				}
			}
		}
	}

	grimoire := &object.Grimoire{
		Name:       node.Name.Value,
		Methods:    methods,
		InitMethod: nil,
		Env:        env,
		Inherits:   parentGrimoire,
		IsArcane:   false,
	}

	if node.Token.Type == token.ARCANE {
		grimoire.IsArcane = true
	}
	if node.InitMethod != nil {
		initFn := &object.Function{
			Parameters: node.InitMethod.Parameters,
			Body:       node.InitMethod.Body,
			Env:        env,
		}
		grimoire.InitMethod = initFn
	}

	env.Set(node.Name.Value, grimoire)
	return grimoire
}

func evalCallExpression(
	fn object.Object,
	args []object.Object,
	env *object.Environment,
) object.Object {
	if len(args) == 1 {
		if tup, ok := args[0].(*object.Tuple); ok {
			args = tup.Elements
		}
	}
	switch fn := fn.(type) {
	case *object.Function:
		globalEnv := getGlobalEnv(fn.Env)
		functionName := "function"
		extendedEnv := extendFunctionEnv(fn, args, globalEnv, functionName)
		evaluated := Eval(fn.Body, extendedEnv)
		return unwrapReturnValue(evaluated)
	case *object.BoundMethod:
		globalEnv := getGlobalEnv(fn.Method.Env)
		functionName := fn.Instance.Grimoire.Name + "." + "method"
		extendedEnv := extendFunctionEnv(fn.Method, args, globalEnv, functionName)
		extendedEnv.Set("self", fn.Instance)
		if fn.Method.IsAbstract {
			return newError("Cannot call abstract method")
		}
		evaluated := Eval(fn.Method.Body, extendedEnv)
		return unwrapReturnValue(evaluated)
	case *object.Grimoire:
		if fn.IsArcane {
			return newError("cannot instantiate arcane grimoire: %s", fn.Name)
		}
		instance := &object.Instance{
			Grimoire: fn,
			Env:      object.NewEnclosedEnvironment(fn.Env),
		}
		if fn.InitMethod != nil {
			globalEnv := getGlobalEnv(fn.Env)
			functionName := fn.Name + ".init"
			extendedEnv := extendFunctionEnv(fn.InitMethod, args, globalEnv, functionName)
			extendedEnv.Set("self", instance)
			Eval(fn.InitMethod.Body, extendedEnv)
		}
		return instance
	case *object.Builtin:
		return fn.Fn(args...)
	default:
		return newError("not a function: %s", fn.Type())
	}
}

func evalDotExpression(node *ast.DotExpression, env *object.Environment) object.Object {
	leftObj := Eval(node.Left, env)
	if isError(leftObj) {
		return leftObj
	}

	if node.Left.String() == "super" {
		instance, ok := env.Get("self")
		if !ok || instance == nil {
			return newError("'super' can only be used in an instance method")
		}

		inst, ok := instance.(*object.Instance)
		if !ok {
			return newError("'super' must be used in an instance of a grimoire")
		}

		if inst.Grimoire == nil || inst.Grimoire.Inherits == nil {
			return newError("no parent class found for 'super'")
		}

		parentMethod, ok := inst.Grimoire.Inherits.Methods[node.Right.Value]
		if !ok {
			return newError("no method '%s' found in parent class", node.Right.Value)
		}
		return &object.BoundMethod{
			Instance: inst,
			Method:   parentMethod,
		}
	}

	instance, ok := leftObj.(*object.Instance)
	if !ok {
		return newError("type error: %s is not an instance", leftObj.Type())
	}

	fieldOrMethodName := node.Right.Value

	if val, found := instance.Env.Get(fieldOrMethodName); found {
		return val
	}

	method, ok := instance.Grimoire.Methods[fieldOrMethodName]
	if !ok {
		return newError("undefined property or method: %s", fieldOrMethodName)
	}

	if method.IsPrivate && !sameClass(env, instance.Grimoire) {
		return newError(
			"private method '%s' not accessible outside its defining class",
			fieldOrMethodName,
		)
	}
	if method.IsProtected && !sameOrSubclass(env, instance.Grimoire) {
		return newError("protected method '%s' not accessible here", fieldOrMethodName)
	}

	return &object.BoundMethod{
		Instance: instance,
		Method:   method,
	}
}

func sameClass(env *object.Environment, target *object.Grimoire) bool {
	callerSelf, ok := env.Get("self")
	if !ok {
		return false
	}
	callerInst, ok := callerSelf.(*object.Instance)
	if !ok {
		return false
	}
	return callerInst.Grimoire == target
}

func sameOrSubclass(env *object.Environment, target *object.Grimoire) bool {
	callerSelf, ok := env.Get("self")
	if !ok {
		return false
	}
	callerInst, ok := callerSelf.(*object.Instance)
	if !ok {
		return false
	}

	grim := callerInst.Grimoire
	for grim != nil {
		if grim == target {
			return true
		}
		grim = grim.Inherits
	}
	return false
}

func evalHashLiteral(
	node *ast.HashLiteral,
	env *object.Environment,
) object.Object {
	pairs := make(map[object.HashKey]object.HashPair)
	for keyNode, valueNode := range node.Pairs {
		key := Eval(keyNode, env)
		if isError(key) {
			return key
		}
		hashKey, ok := key.(object.Hashable)
		if !ok {
			return newError("unusable as hash key: %s", key.Type())
		}
		value := Eval(valueNode, env)
		if isError(value) {
			return value
		}
		hashed := hashKey.HashKey()
		pairs[hashed] = object.HashPair{Key: key, Value: value}
	}
	return &object.Hash{Pairs: pairs}
}

func evalTupleLiteral(tl *ast.TupleLiteral, env *object.Environment) object.Object {
	elements := evalExpressions(tl.Elements, env)
	if len(elements) == 1 && isError(elements[0]) {
		return elements[0]
	}

	return &object.Tuple{Elements: elements}
}

func evalIndexExpression(left, index object.Object) object.Object {
	switch {
	case left.Type() == object.TUPLE_OBJ:
		return evalTupleIndexExpression(left, index)
	case left.Type() == object.ARRAY_OBJ:
		if index.Type() == object.INTEGER_OBJ {
			return evalArrayIndexExpression(left, index)
		} else if index.Type() == object.RANGE_OBJ {
			return evalArraySliceExpression(left, index)
		} else {
			return newError("array index must be INTEGER or RANGE, got %s", index.Type())
		}
	case left.Type() == object.HASH_OBJ:
		return evalHashIndexExpression(left, index)
	default:
		return newError("index operator not supported: %s", left.Type())
	}
}

func evalTupleIndexExpression(tuple, index object.Object) object.Object {
	tupleObj := tuple.(*object.Tuple)
	idx := int(index.(*object.Integer).Value)
	if idx < 0 || idx >= len(tupleObj.Elements) {
		return NONE
	}
	return tupleObj.Elements[idx]
}

func evalHashIndexExpression(hash, index object.Object) object.Object {
	hashObject := hash.(*object.Hash)
	key, ok := index.(object.Hashable)
	if !ok {
		return newError("unusable as hash key: %s", index.Type())
	}
	pair, ok := hashObject.Pairs[key.HashKey()]
	if !ok {
		return NONE
	}
	return pair.Value
}

func evalArrayIndexExpression(array, index object.Object) object.Object {
	arrayObject := array.(*object.Array)
	idx := index.(*object.Integer).Value
	maxIndex := int64(len(arrayObject.Elements) - 1)
	if idx < 0 || idx > maxIndex {
		return NONE
	}
	return arrayObject.Elements[idx]
}

func evalArraySliceExpression(array, rangeObj object.Object) object.Object {
	arrayObject := array.(*object.Array)
	rangeVal := rangeObj.(*object.Range)
	
	// Get start and end values
	var startIdx, endIdx int64
	
	// Handle start index
	if rangeVal.Start == nil || rangeVal.Start.Type() == object.NONE_OBJ {
		startIdx = 0
	} else if rangeVal.Start.Type() == object.INTEGER_OBJ {
		startIdx = rangeVal.Start.(*object.Integer).Value
		if startIdx < 0 {
			startIdx = int64(len(arrayObject.Elements)) + startIdx
		}
	} else {
		return newError("array slice start index must be INTEGER, got %s", rangeVal.Start.Type())
	}
	
	// Handle end index
	if rangeVal.End == nil || rangeVal.End.Type() == object.NONE_OBJ {
		endIdx = int64(len(arrayObject.Elements))
	} else if rangeVal.End.Type() == object.INTEGER_OBJ {
		endIdx = rangeVal.End.(*object.Integer).Value
		if endIdx < 0 {
			endIdx = int64(len(arrayObject.Elements)) + endIdx
		}
	} else {
		return newError("array slice end index must be INTEGER, got %s", rangeVal.End.Type())
	}
	
	// Adjust indices if out of bounds
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > int64(len(arrayObject.Elements)) {
		endIdx = int64(len(arrayObject.Elements))
	}
	if startIdx >= int64(len(arrayObject.Elements)) || endIdx <= 0 || startIdx >= endIdx {
		return &object.Array{Elements: []object.Object{}}
	}
	
	// Create new array with elements from start to end
	newElements := make([]object.Object, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		newElements = append(newElements, arrayObject.Elements[i])
	}
	
	return &object.Array{Elements: newElements}
}

func evalExpressions(exps []ast.Expression, env *object.Environment) []object.Object {
	var result []object.Object

	for _, e := range exps {
		evaluated := Eval(e, env)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

func extendFunctionEnv(
	fn *object.Function,
	args []object.Object,
	global *object.Environment,
	functionName string,
) *object.Environment {
	env := object.NewEnclosedEnvironment(fn.Env)
	
	// Set function name for stack traces
	env.Set("__function_name", &object.String{Value: functionName})

	for i, param := range fn.Parameters {
		if i < len(args) {
			env.Set(param.Name.Value, args[i])
		} else if param.DefaultValue != nil {
			if ident, ok := param.DefaultValue.(*ast.Identifier); ok {
				if val, ok := global.Get(ident.Value); ok {
					env.Set(param.Name.Value, val)
				} else {
					env.Set(param.Name.Value, newError("identifier not found: "+ident.Value))
				}
			} else {
				defaultVal := Eval(param.DefaultValue, fn.Env)
				env.Set(param.Name.Value, defaultVal)
			}
		} else {
			env.Set(param.Name.Value, NONE)
		}
	}

	return env
}

func unwrapReturnValue(obj object.Object) object.Object {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

func evalIdentifier(node *ast.Identifier, env *object.Environment) object.Object {
	// First check builtins.
	if builtin, ok := builtins[node.Value]; ok {
		return builtin
	}
	// Then check the environment.
	if val, ok := env.Get(node.Value); ok {
		return val
	}
	if node.Value == "None" {
		return object.NONE
	}
	return newError("identifier not found: " + node.Value)
}

func evalProgram(program *ast.Program, env *object.Environment) object.Object {
	var result object.Object

	for _, statement := range program.Statements {
		result = Eval(statement, env)

		switch result.(type) {
		case *object.ReturnValue:
			return result.(*object.ReturnValue).Value
		case *object.Error, *object.CustomError:
			return result
		}
	}
	return result
}

func evalBlockStatement(block *ast.BlockStatement, env *object.Environment) object.Object {
	var result object.Object

	for _, statement := range block.Statements {
		result = Eval(statement, env)
		if result != nil {
			rt := result.Type()

			if rt == object.RETURN_VALUE_OBJ ||
				rt == object.ERROR_OBJ ||
				rt == object.CUSTOM_ERROR_OBJ ||
				rt == object.STOP.Type() ||
				rt == object.SKIP.Type() {
				return result
			}
		}
	}

	return result
}

func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

func evalPrefixExpression(
	operator string,
	node *ast.PrefixExpression,
	env *object.Environment,
) object.Object {
	switch operator {
	case "!":
		right := Eval(node.Right, env)
		return evalBangOperatorExpression(right, env)
	case "not":
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalBangOperatorExpression(right, env)
	case "~":
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		intOperand, ok := right.(*object.Integer)
		if !ok {
			return newError("unsupported operand type for ~: %s", right.Type())
		}

		return &object.Integer{Value: ^intOperand.Value}

	case "-":
		right := Eval(node.Right, env)
		return evalMinusPrefixOperatorExpression(right, env)
	default:
		return newError("unknown operator: %s%s", operator, Eval(node.Right, env).Type())
	}
}

func evalInfixExpression(
	operator string,
	left, right object.Object,
) object.Object {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == object.BOOLEAN_OBJ && right.Type() == object.BOOLEAN_OBJ:
		return evalBooleanInfixExpression(operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	case left.Type() == object.ARRAY_OBJ && right.Type() == object.ARRAY_OBJ:
		return evalArrayInfixExpression(operator, left, right)
	case left == object.NONE && right == object.NONE:
		return nativeBoolToBooleanObject(operator == "==")
	case left == object.NONE || right == object.NONE:
		if operator == "==" {
			return nativeBoolToBooleanObject(false)
		} else if operator == "!=" {
			return nativeBoolToBooleanObject(true)
		}
	case left.Type() != right.Type():
		return newError("type mismatch: %s %s %s", left.Type(), operator, right.Type())
	case left.Type() == object.FLOAT_OBJ || right.Type() == object.FLOAT_OBJ:
		leftVal := toFloat(left)
		rightVal := toFloat(right)
		switch operator {
		case "+":
			return &object.Float{Value: leftVal + rightVal}
		case "-":
			return &object.Float{Value: leftVal - rightVal}
		case "*":
			return &object.Float{Value: leftVal * rightVal}
		case "/":
			return &object.Float{Value: leftVal / rightVal}
		case "**":
			return &object.Float{Value: math.Pow(leftVal, rightVal)}
		default:
			return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
		}
	}

	return newError(
		"unknown operator or type mismatch: %s %s %s",
		left.Type(),
		operator,
		right.Type(),
	)
}

func toFloat(obj object.Object) float64 {
	switch obj := obj.(type) {
	case *object.Integer:
		return float64(obj.Value)
	case *object.Float:
		return obj.Value
	default:
		return 0.0
	}
}

func evalStringInfixExpression(
	operator string,
	left, right object.Object,
) object.Object {
	if operator != "+" {
		return newError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value
	return &object.String{Value: leftVal + rightVal}
}

func evalArrayInfixExpression(
	operator string,
	left, right object.Object,
) object.Object {
	if operator != "+" {
		return newError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
	leftVal := left.(*object.Array)
	rightVal := right.(*object.Array)
	
	// Create a new array with the combined elements
	newElements := make([]object.Object, len(leftVal.Elements)+len(rightVal.Elements))
	copy(newElements, leftVal.Elements)
	copy(newElements[len(leftVal.Elements):], rightVal.Elements)
	
	return &object.Array{Elements: newElements}
}

func evalBooleanInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Boolean).Value
	rightVal := right.(*object.Boolean).Value
	switch operator {
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalPrefixIncrementDecrement(
	operator string,
	node *ast.PrefixExpression,
	env *object.Environment,
) object.Object {
	switch operand := node.Right.(type) {
	case *ast.Identifier:
		obj, ok := env.Get(operand.Value)
		if !ok {
			return newError("undefined variable '%s'", operand.Value)
		}

		intObj, ok := obj.(*object.Integer)
		if !ok {
			return newError("prefix '%s' operator requires an integer variable '%s'", operator, operand.Value)
		}

		if operator == "++" {
			intObj.Value += 1
		} else if operator == "--" {
			intObj.Value -= 1
		}

		env.Set(operand.Value, intObj)
		return intObj

	default:
		return newError("prefix '%s' operator requires an integer or identifier", operator)
	}
}

func evalPostfixIncrementDecrement(
	operator string,
	node *ast.PostfixExpression,
	env *object.Environment,
) object.Object {
	switch operand := node.Left.(type) {
	case *ast.Identifier:

		obj, ok := env.Get(operand.Value)
		if !ok {
			return newError("undefined variable '%s'", operand.Value)
		}

		intObj, ok := obj.(*object.Integer)
		if !ok {
			return newError("postfix '%s' operator requires an integer variable '%s'", operator, operand.Value)
		}

		oldValue := intObj.Value

		var newValue int64
		if operator == "++" {
			newValue = oldValue + 1
		} else if operator == "--" {
			newValue = oldValue - 1
		}

		newObj := &object.Integer{Value: newValue}

		env.Set(operand.Value, newObj)

		return &object.Integer{Value: oldValue}
	default:
		return newError("postfix '%s' operator requires an integer or identifier", operator)
	}
}

func evalPostfixExpression(
	operator string,
	node *ast.PostfixExpression,
	env *object.Environment,
) object.Object {
	switch operator {
	case "++", "--":
		return evalPostfixIncrementDecrement(operator, node, env)
	default:
		return newError("unknown operator: %s", operator)
	}
}

func evalBangOperatorExpression(right object.Object, env *object.Environment) object.Object {
	switch right {
	case TRUE:
		return FALSE
	case FALSE:
		return TRUE
	case NONE:
		return TRUE
	default:
		return FALSE
	}
}

func evalMinusPrefixOperatorExpression(right object.Object, env *object.Environment) object.Object {
	if right.Type() != object.INTEGER_OBJ && right.Type() != object.FLOAT_OBJ {
		return newError("unknown operator: -%s", right.Type())
	}
	switch right := right.(type) {
	case *object.Integer:
		return &object.Integer{Value: -right.Value}
	case *object.Float:
		return &object.Float{Value: -right.Value}
	default:
		return newError("unknown type for minus operator: %s", right.Type())
	}
}

func evalIncrementOperatorExpression(side object.Object) object.Object {
	if side.Type() != object.INTEGER_OBJ {
		return NONE
	}
	value := side.(*object.Integer).Value
	return &object.Integer{Value: value + 1}
}

func evalDecrementOperatorExpression(side object.Object) object.Object {
	if side.Type() != object.INTEGER_OBJ {
		return NONE
	}
	value := side.(*object.Integer).Value
	return &object.Integer{Value: value - 1}
}

func evalIntegerInfixExpression(
	operator string,
	left, right object.Object,
) object.Object {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value
	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		return &object.Integer{Value: leftVal / rightVal}
	case "%":
		return &object.Integer{Value: leftVal % rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "**":
		return &object.Integer{Value: int64(math.Pow(float64(leftVal), float64(rightVal)))}
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)

	case "<<":
		return &object.Integer{Value: leftVal << uint(rightVal)}
	case ">>":
		return &object.Integer{Value: leftVal >> uint(rightVal)}
	case "&":
		return &object.Integer{Value: leftVal & rightVal}
	case "^":
		return &object.Integer{Value: leftVal ^ rightVal}
	case "|":
		return &object.Integer{Value: leftVal | rightVal}

	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalCompoundAssignment(node *ast.InfixExpression, env *object.Environment) object.Object {
	rightVal := Eval(node.Right, env)
	if isError(rightVal) {
		return rightVal
	}

	switch leftNode := node.Left.(type) {
	case *ast.Identifier:

		currVal, ok := env.Get(leftNode.Value)
		if !ok {
			return newError("undefined variable: %s", leftNode.Value)
		}

		newVal := applyCompoundOperator(node.Operator, currVal, rightVal)
		if isError(newVal) {
			return newVal
		}

		env.Set(leftNode.Value, newVal)
		return newVal

	default:
		return newError("invalid assignment target: %T", leftNode)
	}
}

func applyCompoundOperator(operator string, leftVal, rightVal object.Object) object.Object {
	switch l := leftVal.(type) {
	case *object.Integer:
		rInt, ok := rightVal.(*object.Integer)
		if !ok {
			return newError("type mismatch: expected INTEGER, got %s", rightVal.Type())
		}
		switch operator {
		case "+=":
			return &object.Integer{Value: l.Value + rInt.Value}
		case "-=":
			return &object.Integer{Value: l.Value - rInt.Value}
		case "*=":
			return &object.Integer{Value: l.Value * rInt.Value}
		case "/=":
			if rInt.Value == 0 {
				return newError("division by zero")
			}
			return &object.Integer{Value: l.Value / rInt.Value}
		default:
			return newError("unknown operator: %s", operator)
		}

	case *object.Float:
		rFloat, ok := rightVal.(*object.Float)
		if !ok {
			return newError("type mismatch: expected FLOAT, got %s", rightVal.Type())
		}
		switch operator {
		case "+=":
			return &object.Float{Value: l.Value + rFloat.Value}
		case "-=":
			return &object.Float{Value: l.Value - rFloat.Value}
		case "*=":
			return &object.Float{Value: l.Value * rFloat.Value}
		case "/=":
			if rFloat.Value == 0 {
				return newError("division by zero")
			}
			return &object.Float{Value: l.Value / rFloat.Value}
		default:
			return newError("unknown operator: %s", operator)
		}

	default:
		return newError("unsupported type for compound assignment: %s", leftVal.Type())
	}
}

func evalIfExpression(ie *ast.IfStatement, env *object.Environment) object.Object {
	condition := Eval(ie.Condition, env)
	if isTruthy(condition) {
		return Eval(ie.Consequence, env)
	}

	for _, branch := range ie.OtherwiseBranches {
		condition = Eval(branch.Condition, env)
		if isError(condition) {
			return condition
		}
		if isTruthy(condition) {
			return Eval(branch.Consequence, env)
		}
	}

	if ie.Alternative != nil {
		return Eval(ie.Alternative, env)
	}

	return NONE
}

func newError(format string, a ...interface{}) *object.Error {
	return object.NewError(fmt.Sprintf(format, a...))
}

func isError(obj object.Object) bool {
	if obj == nil {
		return false
	}
	return obj.Type() == object.ERROR_OBJ || obj.Type() == object.CUSTOM_ERROR_OBJ
}

func evalWhileStatement(node *ast.WhileStatement, env *object.Environment) object.Object {
	for {

		condition := Eval(node.Condition, env)
		if isError(condition) {
			return condition
		}
		if !isTruthy(condition) {
			break
		}

		n := len(node.Body.Statements)
		var controlSignal object.Object = nil

		for i := 0; i < n-1; i++ {
			res := Eval(node.Body.Statements[i], env)

			rt := res.Type()
			if rt == object.STOP.Type() || rt == object.SKIP.Type() ||
				rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ || rt == object.CUSTOM_ERROR_OBJ {
				controlSignal = res
				break
			}
		}

		if n > 0 {
			_ = Eval(node.Body.Statements[n-1], env)
		}

		if controlSignal != nil {
			rt := controlSignal.Type()
			if rt == object.STOP.Type() {
				break
			}
			if rt == object.SKIP.Type() {
				continue
			}
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ ||
				rt == object.CUSTOM_ERROR_OBJ {
				return controlSignal
			}
		}
	}
	return NONE
}

func isTruthy(obj object.Object) bool {
	switch obj := obj.(type) {
	case *object.Boolean:
		return obj.Value
	case *object.String:
		return len(obj.Value) > 0
	case *object.Array:
		return len(obj.Elements) > 0
	case *object.Tuple:
		return len(obj.Elements) > 0
	case *object.Hash:
		return len(obj.Pairs) > 0
	case *object.None:
		return false
	default:
		return true
	}
}

func evalForStatement(fs *ast.ForStatement, env *object.Environment) object.Object {
	iterable := Eval(fs.Iterable, env)
	if isError(iterable) {
		return iterable
	}

	var result object.Object = NONE

	switch iter := iterable.(type) {
	case *object.Array:
		for _, elem := range iter.Elements {

			switch varExpr := fs.Variable.(type) {
			case *ast.Identifier:

				env.Set(varExpr.Value, elem)
			case *ast.TupleLiteral:

				var items []object.Object
				if tupObj, ok := elem.(*object.Tuple); ok {
					items = tupObj.Elements
				} else if arrObj, ok := elem.(*object.Array); ok {
					items = arrObj.Elements
				} else {
					return newError("cannot unpack non-iterable element: %s", elem.Type())
				}
				if len(varExpr.Elements) != len(items) {
					return newError("unpacking mismatch: expected %d values, got %d", len(varExpr.Elements), len(items))
				}
				for i, target := range varExpr.Elements {

					ident, ok := target.(*ast.Identifier)
					if !ok {
						return newError("invalid assignment target in for loop")
					}
					env.Set(ident.Value, items[i])
				}
			default:

				env.Set(fs.Variable.String(), elem)
			}

			for _, stmt := range fs.Body.Statements {
				result = Eval(stmt, env)
				rt := result.Type()
				if rt == object.STOP.Type() {
					return NONE
				}
				if rt == object.SKIP.Type() {
					break
				}
				if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ || rt == object.CUSTOM_ERROR_OBJ {
					return result
				}
			}
		}
	default:
		return newError("unsupported iterable type: %s", iterable.Type())
	}

	if fs.Alternative != nil {
		result = Eval(fs.Alternative, env)
	}
	return result
}

func evalImportStatement(node *ast.ImportStatement, env *object.Environment) object.Object {
	filePath := node.FilePath.Value + ".crl"

	if importedFiles[filePath] {
		return object.NONE
	}
	importedFiles[filePath] = true

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return newError("could not import file: %s", err)
	}

	// Create lexer and parser with filename for better error reporting
	l := lexer.New(string(fileContent), filePath)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		return newError("parsing errors in imported file: %v", p.Errors())
	}

	importEnv := object.NewEnclosedEnvironment(env)
	Eval(program, importEnv)

	namespace := &object.Namespace{Env: importEnv}

	if node.Alias != nil {
		env.Set(node.Alias.Value, namespace)
	} else {
		for _, name := range importEnv.GetNames() {
			val, _ := importEnv.Get(name)
			if val.Type() == object.GRIMOIRE_OBJ {
				env.Set(name, val)
			}
		}
	}

	return object.NONE
}
