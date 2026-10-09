package dart

func skipMetadata(toks []token, at int) int {
	for at < len(toks) && toks[at].text == "@" {
		at += 2
		for at+1 < len(toks) && toks[at].text == "." && toks[at+1].kind == tokIdent {
			at += 2
		}

		if at < len(toks) && toks[at].text == "(" {
			at = closing(toks, at) + 1
		}
	}

	return at
}

// closing returns the index of the bracket that closes the one at open.
func closing(toks []token, open int) int {
	depth := 0

	for index := open; index < len(toks); index++ {
		switch toks[index].text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--

			if depth == 0 {
				return index
			}
		}
	}

	return len(toks) - 1
}

// identBefore finds the name a "(" belongs to, skipping type arguments.
func identBefore(toks []token, paren int) int {
	at := paren - 1
	if at >= 0 && toks[at].text == ">" {
		depth := 0

		for ; at >= 0; at-- {
			switch toks[at].text {
			case ">":
				depth++
			case "<":
				depth--
			}

			if depth == 0 {
				break
			}
		}

		at--
	}

	return at
}
