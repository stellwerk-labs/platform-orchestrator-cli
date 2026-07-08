package ref

func DerefOr[T any](v *T, d T) T {
	if v == nil {
		return d
	}
	return *v
}

func RefStringEmptyNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func Ref[a any](i a) *a {
	return &i
}
