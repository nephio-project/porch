package set_namespace

import "github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"

func NestedStringOrDie(o *fn.KubeObject, fields ...string) string {
	val, _, err := o.NestedString(fields...)
	if err != nil {
		panic(err)
	}
	return val
}

func SetNestedStringOrDie(o *fn.SubObject, value string, fields ...string) {
	err := o.SetNestedString(value, fields...)
	if err != nil {
		panic(err)
	}
}
