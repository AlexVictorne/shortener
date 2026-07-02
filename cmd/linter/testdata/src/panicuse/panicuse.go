package panicuse

func Boom() {
	panic("something went wrong") // want `use of panic is forbidden`
}

func AlsoBoom(ok bool) {
	if !ok {
		panic("not ok") // want `use of panic is forbidden`
	}
}
