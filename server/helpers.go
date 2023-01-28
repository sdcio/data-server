package server

import (
	"strings"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

// path elements to strings, no keys
func PathToStrings(p *schemapb.Path) []string {
	numElem := len(p.GetElem())
	if numElem == 0 {
		return nil
	}
	rs := make([]string, 0, numElem)
	sb := &strings.Builder{}
	for _, pe := range p.GetElem() {
		sb.Reset()
		sb.WriteString(pe.GetName())
		rs = append(rs, sb.String())
	}
	return rs
}
