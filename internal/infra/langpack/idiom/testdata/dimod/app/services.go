package app

type S01 struct{}

func NewS01() *S01 { return &S01{} }

type S02 struct{ prev *S01 }

func NewS02(prev *S01) *S02 { return &S02{prev: prev} }

type S03 struct{ prev *S02 }

func NewS03(prev *S02) *S03 { return &S03{prev: prev} }

type S04 struct{ prev *S03 }

func NewS04(prev *S03) *S04 { return &S04{prev: prev} }

type S05 struct{ prev *S04 }

func NewS05(prev *S04) *S05 { return &S05{prev: prev} }

type S06 struct{ prev *S05 }

func NewS06(prev *S05) *S06 { return &S06{prev: prev} }

type S07 struct{ prev *S06 }

func NewS07(prev *S06) *S07 { return &S07{prev: prev} }

type S08 struct{ prev *S07 }

func NewS08(prev *S07) *S08 { return &S08{prev: prev} }

type S09 struct{ prev *S08 }

func NewS09(prev *S08) *S09 { return &S09{prev: prev} }

type S10 struct{ prev *S09 }

func NewS10(prev *S09) *S10 { return &S10{prev: prev} }

type S11 struct{ prev *S10 }

func NewS11(prev *S10) *S11 { return &S11{prev: prev} }

type S12 struct {
	prev *S11
	root *S01
}

func NewS12(prev *S11, root *S01) *S12 { return &S12{prev: prev, root: root} }
