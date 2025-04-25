package evaluator

import (
	"github.com/javanhut/Carrion/src/ast"
	"github.com/javanhut/Carrion/src/object"
)

// getArrayFromObject extracts an array from an object, regardless of whether
// it's a primitive array or a wrapped array in an Array grimoire instance
func getArrayFromObject(obj object.Object) *object.Array {
	switch obj := obj.(type) {
	case *object.Array:
		return obj
	case *object.Instance:
		if obj.Grimoire != nil && obj.Grimoire.Name == "Array" {
			elementsObj, ok := obj.Env.Get("elements")
			if !ok {
				return nil
			}
			
			switch elementsObj := elementsObj.(type) {
			case *object.Array:
				return elementsObj
			case *object.Instance:
				return getArrayFromObject(elementsObj)
			}
		}
	}
	return nil
}

// isArrayGrimoireInstance checks if an object is an instance of Array grimoire
func isArrayGrimoireInstance(obj object.Object) bool {
	if instance, ok := obj.(*object.Instance); ok {
		return instance.Grimoire != nil && instance.Grimoire.Name == "Array"
	}
	return false
}

// getGlobalEnv returns the outermost (global) environment
func getGlobalEnv(env *object.Environment) *object.Environment {
	for env.GetOuter() != nil {
		env = env.GetOuter()
	}
	return env
}

// extendFunctionEnv creates a new environment for a function call
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
					env.Set(param.Name.Value, &object.Error{Message: "identifier not found: " + ident.Value})
				}
			} else {
				// We can't call Eval here due to circular imports
				// Instead, we'll set NONE as a fallback
				env.Set(param.Name.Value, &object.None{Value: "None"})
			}
		} else {
			env.Set(param.Name.Value, &object.None{Value: "None"})
		}
	}

	return env
}

// wrapPrimitiveWithGrimoire wraps a primitive type with its corresponding grimoire instance
// This allows primitive types to have access to methods defined in their grimoire definitions
func wrapPrimitiveWithGrimoire(obj object.Object, env *object.Environment) object.Object {
	// If the object is already an instance, don't wrap it again
	if _, isInstance := obj.(*object.Instance); isInstance {
		return obj
	}
	
	switch obj := obj.(type) {
	case *object.Array:
		// Skip wrapping if we have no environment to look up the grimoire
		if env == nil {
			return obj
		}
		
		// Get the global environment to ensure we find global definitions
		globalEnv := getGlobalEnv(env)
		
		// Get the Array grimoire from the environment
		arrayGrimoire, ok := globalEnv.Get("Array")
		if !ok {
			return obj // If Array grimoire not found, return the original array
		}
		
		// Check if it's actually a grimoire
		grimoire, ok := arrayGrimoire.(*object.Grimoire)
		if !ok {
			return obj // If not a grimoire, return the original array
		}
		
		// Create a new instance of the Array grimoire
		instance := &object.Instance{
			Grimoire: grimoire,
			Env:      object.NewEnclosedEnvironment(grimoire.Env),
		}
		
		// Set the elements field directly in the instance environment
		instance.Env.Set("elements", obj)
		
		// Initialize the instance with the original array elements
		if grimoire.InitMethod != nil {
			// Create extended environment for init method
			globalEnv := getGlobalEnv(grimoire.Env)
			functionName := grimoire.Name + ".init"
			extendedEnv := extendFunctionEnv(grimoire.InitMethod, []object.Object{obj}, globalEnv, functionName)
			extendedEnv.Set("self", instance)
			
			// Since we can't call Eval directly due to circular imports,
			// we'll skip the evaluation step and just set the elements directly
			extendedEnv.Set("elements", obj)
		}
		
		// Return the instance that wraps the array
		return instance
	
	// Add other primitive types here as needed, such as:
	// case *object.String:
	// case *object.Integer:
	// etc.
	
	default:
		return obj // Return the original object for other types
	}
}

// combineArrays creates a new array by combining elements from two arrays or array instances
func combineArrays(left, right object.Object, env *object.Environment) object.Object {
	// Extract arrays from objects (whether they're primitives or instances)
	leftArray := getArrayFromObject(left)
	rightArray := getArrayFromObject(right)
	
	if leftArray == nil || rightArray == nil {
		return nil
	}
	
	// Create a new array with the combined elements
	newElements := make([]object.Object, len(leftArray.Elements)+len(rightArray.Elements))
	copy(newElements, leftArray.Elements)
	copy(newElements[len(leftArray.Elements):], rightArray.Elements)
	
	// Create a new array object
	arrayObj := &object.Array{Elements: newElements}
	
	// Get the environment for wrapping
	var wrapEnv *object.Environment
	if instance, ok := left.(*object.Instance); ok {
		wrapEnv = instance.Env
	} else if instance, ok := right.(*object.Instance); ok {
		wrapEnv = instance.Env
	} else {
		wrapEnv = env
	}
	
	// Wrap it with grimoire and return
	return wrapPrimitiveWithGrimoire(arrayObj, wrapEnv)
}