package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

type CXLine struct {
	FileName   SourceFileGid
	LineNumber int
	LineStr    stringpool.StringGid
}
