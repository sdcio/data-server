package types

import "github.com/beevik/etree"

type NetconfResponse struct {
	Doc *etree.Document
}

func NewNetconfResponse(doc *etree.Document) *NetconfResponse {
	return &NetconfResponse{
		Doc: doc,
	}
}

func (nr *NetconfResponse) DocAsString() string {
	if nr.Doc == nil {
		return ""
	}
	s, _ := nr.Doc.WriteToString()
	return s
}
