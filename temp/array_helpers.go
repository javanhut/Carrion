// getArrayFromObject extracts an array from an object, regardless of whether
// it's a primitive array or a wrapped array in an Array grimoire instance
func getArrayFromObject(obj object.Object) *object.Array {
	switch obj := obj.(type) {
	case *object.Array:
		return obj
	case *object.Instance:
		if obj.Grimoire \!= nil && obj.Grimoire.Name == "Array" {
			if elementsObj, ok := obj.Env.Get("elements"); ok {
				if array, ok := elementsObj.(*object.Array); ok {
					return array
				}
			}
		}
	}
	return nil
}
