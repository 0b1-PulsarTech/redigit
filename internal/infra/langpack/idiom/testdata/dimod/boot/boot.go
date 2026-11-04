package boot

import (
	"example.com/dimod/app"
	"example.com/dimod/inj"
)

func NewInjector() *inj.Injector {
	in := inj.New()

	inj.Register2(in, app.NewS12)
	inj.Register1(in, app.NewS11)
	inj.Register1(in, app.NewS10)
	inj.Register1(in, app.NewS09)
	inj.Register1(in, app.NewS08)
	inj.Register1(in, app.NewS07)
	inj.Register1(in, app.NewS06)
	inj.Register1(in, app.NewS05)
	inj.Register1(in, app.NewS04)
	inj.Register1(in, app.NewS03)
	inj.Register1(in, app.NewS02)
	inj.Register0(in, app.NewS01)

	return in
}

func Boot() *app.S12 {
	return inj.Get[*app.S12](NewInjector())
}
