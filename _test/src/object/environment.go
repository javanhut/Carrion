package object

// environment.go
type Environment struct {
	store map[string]Object
	outer *Environment
}

func NewEnvironment() *Environment {
	s := make(map[string]Object)
	return &Environment{store: s, outer: nil}
}

func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}

func (e *Environment) Get(name string) (Object, bool) {
	obj, ok := e.store[name]
	if !ok && e.outer != nil {
		obj, ok = e.outer.Get(name)
	}
	return obj, ok
}

func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
	return val
}

func (e *Environment) GetNames() []string {
	names := make([]string, 0)
	for name := range e.store {
		names = append(names, name)
	}
	return names
}

func (e *Environment) GetOuter() *Environment {
	return e.outer
}

// GetFunctionName tries to determine the current function name
// Returns empty string if not in a function/method
func (e *Environment) GetFunctionName() string {
	// Check if we're in a method
	if self, ok := e.Get("self"); ok {
		if instance, ok := self.(*Instance); ok {
			return instance.Grimoire.Name + " method"
		}
	}
	
	// Check for function name
	if fnName, ok := e.Get("__function_name"); ok {
		if str, ok := fnName.(*String); ok {
			return str.Value
		}
	}
	
	// Check outer environments
	if e.outer != nil {
		return e.outer.GetFunctionName()
	}
	
	return ""
}
