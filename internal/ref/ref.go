package ref

func New[T any](v *T) Ref[T] {
	if v == nil {
		var zero T
		return Ref[T]{ptr: &zero}
	}
	return Ref[T]{ptr: v}
}

type Ref[T any] struct {
	ptr *T
}

func (r Ref[T]) Val() T {
	return *r.ptr
}
